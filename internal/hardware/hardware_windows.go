//go:build windows

package hardware

import (
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func cpuName() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\CentralProcessor\0`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	name, _, _ := k.GetStringValue("ProcessorNameString")
	return strings.Join(strings.Fields(name), " ")
}

var iidFactory1 = windows.GUID{Data1: 0x770aae78, Data2: 0xf26f, Data3: 0x4dba, Data4: [8]byte{0xa8, 0x29, 0x25, 0x3c, 0x83, 0xd1, 0xb3, 0x87}}

// DXGI_ADAPTER_DESC1
type adapterDesc1 struct {
	Description           [128]uint16
	VendorID              uint32
	DeviceID              uint32
	SubSysID              uint32
	Revision              uint32
	DedicatedVideoMemory  uintptr
	DedicatedSystemMemory uintptr
	SharedSystemMemory    uintptr
	LuidLow               uint32
	LuidHigh              int32
	Flags                 uint32
}

const adapterFlagSoftware = 2 // DXGI_ADAPTER_FLAG_SOFTWARE (Microsoft Basic Render Driver)

// vtable slots used below.
const (
	slotRelease       = 2
	slotEnumAdapters1 = 12 // IDXGIFactory1
	slotGetDesc1      = 10 // IDXGIAdapter1
)

// gpus lists the display adapters through DXGI. Unlike the registry, this only
// returns adapters that are actually present.
func gpus() []GPU {
	create := windows.NewLazySystemDLL("dxgi.dll").NewProc("CreateDXGIFactory1")
	if create.Find() != nil {
		return nil
	}
	var factory *comObject
	if hr, _, _ := create.Call(uintptr(unsafe.Pointer(&iidFactory1)), uintptr(unsafe.Pointer(&factory))); hr != 0 || factory == nil {
		return nil
	}
	defer factory.call(slotRelease)

	var out []GPU
	seen := map[string]bool{}
	for i := uintptr(0); i < 16; i++ {
		var adapter *comObject
		if hr := factory.call(slotEnumAdapters1, i, uintptr(unsafe.Pointer(&adapter))); hr != 0 || adapter == nil {
			break // DXGI_ERROR_NOT_FOUND: no more adapters
		}
		var d adapterDesc1
		hr := adapter.call(slotGetDesc1, uintptr(unsafe.Pointer(&d)))
		adapter.call(slotRelease)
		if hr != 0 || d.Flags&adapterFlagSoftware != 0 {
			continue
		}
		name := strings.TrimSpace(windows.UTF16ToString(d.Description[:]))
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, GPU{Name: name, Vendor: vendorOf(d.VendorID, name), VRAMMB: int(d.DedicatedVideoMemory >> 20)})
	}
	return out
}

// comObject is a COM interface pointer: its first field is the vtable.
type comObject struct{ vtbl *[16]uintptr }

func (o *comObject) call(slot int, args ...uintptr) uintptr {
	hr, _, _ := syscall.SyscallN(o.vtbl[slot], append([]uintptr{uintptr(unsafe.Pointer(o))}, args...)...)
	return hr
}
