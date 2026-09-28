//go:build windows

package encoder

import (
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const createNoWindow = 0x08000000

func configureProc(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}

var (
	jobOnce sync.Once
	jobH    windows.Handle
	jobErr  error
)

// initJob creates a Job Object with KILL_ON_JOB_CLOSE. Every ffmpeg is added
// to it, so Windows kills them all when the app exits, even on a crash.
func initJob() {
	h, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		jobErr = err
		return
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	_, err = windows.SetInformationJobObject(
		h,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	)
	if err != nil {
		jobErr = err
		return
	}
	jobH = h
}

func assignToJob(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	jobOnce.Do(initJob)
	if jobErr != nil || jobH == 0 {
		return
	}
	ph, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		return
	}
	defer windows.CloseHandle(ph)
	_ = windows.AssignProcessToJobObject(jobH, ph)
}

// killTree kills the process and its children, in case what we launched was a
// wrapper around the real ffmpeg.
func killTree(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	tk := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
	configureProc(tk)
	_ = tk.Run()
	_ = cmd.Process.Kill() // if taskkill failed
}
