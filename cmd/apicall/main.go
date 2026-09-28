// Command apicall sends one webcast API request as an account. Handy for trying
// out endpoints.
//
//	LASTEDLIVE_DATA=./data go run ./cmd/apicall <account> GET|POST|JSON <path> [k=v ...]
//
// GET sends the k=v pairs as query params, POST as a form, JSON as a JSON
// object (values that are valid JSON are sent as such). {uid} and {sec} in a
// value are replaced with the account's user id and sec_user_id. Output is cut
// at 3000 bytes unless APICALL_FULL is set. In Git Bash, set
// MSYS_NO_PATHCONV=1 so the path isn't rewritten.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"lastedlive/internal/manager"
	"lastedlive/internal/store"
	"lastedlive/internal/tiktok"
)

func main() {
	if len(os.Args) < 4 {
		fmt.Println("usage: apicall <account> GET|POST|JSON <path> [k=v ...]")
		os.Exit(2)
	}
	st, err := store.New()
	must(err)
	m := manager.New(st, "")
	a := st.Get(os.Args[1])
	if a == nil {
		must(fmt.Errorf("no account %q", os.Args[1]))
	}
	cl, err := m.ClientFor(a)
	must(err)

	kv := map[string]string{}
	js := map[string]any{}
	for _, p := range os.Args[4:] {
		k, v, _ := strings.Cut(p, "=")
		v = strings.NewReplacer("{uid}", a.UserID, "{sec}", a.SecUserID).Replace(v)
		kv[k] = v
		var j any
		if json.Unmarshal([]byte(v), &j) == nil {
			js[k] = j
		} else {
			js[k] = v
		}
	}
	url := tiktok.BaseWebcast + os.Args[3]
	var resp map[string]any
	switch strings.ToUpper(os.Args[2]) {
	case "GET":
		resp, err = cl.Get(url, &tiktok.Req{Params: kv})
	case "POST":
		resp, err = cl.Post(url, &tiktok.Req{Form: kv})
	case "JSON":
		resp, err = cl.Post(url, &tiktok.Req{JSON: js})
	}
	out, _ := json.Marshal(resp)
	if len(out) > 3000 && os.Getenv("APICALL_FULL") == "" {
		out = append(out[:3000], "…"...)
	}
	fmt.Printf("err: %v\n%s\n", err, out)
}

func must(err error) {
	if err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
}
