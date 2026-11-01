// Local deployment fixture: empty read-only panel API and an Xray StatsService.
// This program has no access to production credentials or infrastructure.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

func main() {
	if len(os.Args) > 1 {
		if err := verify(os.Args[1]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	server := grpc.NewServer()
	server.RegisterService(&grpc.ServiceDesc{
		ServiceName: "xray.app.stats.command.StatsService", HandlerType: (*interface{})(nil),
		Methods: []grpc.MethodDesc{{MethodName: "QueryStats", Handler: func(_ any, _ context.Context, decode func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
			request := new(emptypb.Empty)
			if err := decode(request); err != nil {
				return nil, err
			}
			return new(emptypb.Empty), nil
		}}},
	}, struct{}{})
	listener, err := net.Listen("tcp", "127.0.0.1:18085")
	if err != nil {
		panic(err)
	}
	go server.Serve(listener)
	http.HandleFunc("/api/nodes", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			w.WriteHeader(405)
			return
		}
		fmt.Fprint(w, "{\"response\":[]}")
	})
	http.HandleFunc("/api/users", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			w.WriteHeader(405)
			return
		}
		fmt.Fprint(w, "{\"response\":{\"users\":[],\"total\":0}}")
	})
	if err := http.ListenAndServe("127.0.0.1:18081", nil); err != nil {
		panic(err)
	}
}

func verify(mode string) error {
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 10 * time.Second}
	call := func(method, path, body string, out any) error {
		request, _ := http.NewRequest(method, "http://127.0.0.1:18080"+path, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-SubGate-CSRF", "1")
		resp, err := client.Do(request)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("%s returned %d", path, resp.StatusCode)
		}
		if out != nil {
			return json.NewDecoder(resp.Body).Decode(out)
		}
		return nil
	}
	if err := call("POST", "/api/v1/auth/login", "{\"username\":\"admin\",\"password\":\"subgate-demo\"}", nil); err != nil {
		return err
	}
	if mode == "token" {
		var result struct {
			Token string `json:"token"`
		}
		if err := call("POST", "/api/v1/agents/enrollment-tokens", "{\"node_id\":\"demo-node-1\"}", &result); err != nil {
			return err
		}
		fmt.Print(result.Token)
		return nil
	}
	var result struct {
		Data []struct {
			NodeID      string `json:"node_id"`
			Online      bool   `json:"online"`
			Diagnostics struct {
				StatsError      string `json:"stats_error"`
				LogsError       string `json:"logs_error"`
				StatsIntervalMS int64  `json:"stats_interval_ms"`
			} `json:"diagnostics"`
		} `json:"data"`
	}
	deadline := time.Now().Add(45 * time.Second)
	for {
		if err := call("GET", "/api/v1/agents", "", &result); err != nil {
			return err
		}
		for _, agent := range result.Data {
			if agent.NodeID == "demo-node-1" && agent.Online && agent.Diagnostics.StatsError == "" && agent.Diagnostics.LogsError == "" && agent.Diagnostics.StatsIntervalMS > 0 {
				return nil
			}
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("no online Agent with working sources")
}
