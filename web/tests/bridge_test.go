//go:build cgo && (darwin || linux)

package tests

import (
	"encoding/json"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestBridge(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ctx := newJSContext()
	defer ctx.close()
	loadBridgeFixture(t, ctx)
	namesJSON, err := ctx.eval("JSON.stringify(cases.map(entry=>entry.name))")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	if err := json.Unmarshal([]byte(namesJSON), &names); err != nil {
		t.Fatal(err)
	}
	if len(names) != 13 {
		t.Fatalf("expected 13 bridge regressions, got %d", len(names))
	}
	for index, name := range names {
		t.Run(name, func(t *testing.T) {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			ctx := newJSContext()
			defer ctx.close()
			loadBridgeFixture(t, ctx)
			encoded, _ := json.Marshal(index)
			if err := ctx.run("cases["+string(encoded)+"].body()", 2*time.Second); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func loadBridgeFixture(t *testing.T, ctx *jsContext) {
	t.Helper()
	for name, path := range map[string]string{"bridgeSource": "../static/js/bridge.js", "shellSource": "../index.html"} {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(string(source))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ctx.eval("globalThis." + name + "=" + string(encoded)); err != nil {
			t.Fatal(err)
		}
	}
	fixture, err := os.ReadFile("bridge.test.js")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ctx.eval(string(fixture)); err != nil {
		t.Fatal(err)
	}
}

func TestJavaScriptRunnerReportsFailures(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ctx := newJSContext()
	defer ctx.close()
	for _, expression := range []string{
		"(()=>{throw new Error('negative control')})()",
		"Promise.resolve().then(()=>{throw new Error('negative control')})",
	} {
		if err := ctx.run(expression, time.Second); err == nil || !strings.Contains(err.Error(), "negative control") {
			t.Fatalf("runner must report assertion/rejection: %v", err)
		}
	}
	if err := ctx.run("new Promise(()=>{})", 20*time.Millisecond); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("runner must reject an unfinished test: %v", err)
	}
	if _, err := ctx.eval("throw new Error('evaluation control')"); err == nil || !strings.Contains(err.Error(), "evaluation control") {
		t.Fatalf("runner must report evaluation exceptions: %v", err)
	}
}
