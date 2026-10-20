//go:build cgo && (darwin || linux)

package tests

/*
#cgo darwin LDFLAGS: -framework JavaScriptCore
#cgo linux pkg-config: javascriptcoregtk-6.0
#include <stdlib.h>
#include <string.h>
#ifdef __APPLE__
#include <JavaScriptCore/JavaScript.h>
typedef JSGlobalContextRef dub_context;
static dub_context dub_create(void) { return JSGlobalContextCreate(NULL); }
static void dub_release(dub_context ctx) { JSGlobalContextRelease(ctx); }
static char *dub_string(dub_context ctx, JSValueRef value) {
 JSStringRef text = JSValueToStringCopy(ctx, value, NULL);
 if (!text) return strdup("JavaScript value cannot be converted to a string");
 size_t size = JSStringGetMaximumUTF8CStringSize(text);
 char *result = malloc(size);
 JSStringGetUTF8CString(text, result, size);
 JSStringRelease(text);
 return result;
}
static char *dub_eval(dub_context ctx, const char *source, char **error) {
 JSStringRef script = JSStringCreateWithUTF8CString(source);
 JSValueRef exception = NULL;
 JSValueRef result = JSEvaluateScript(ctx, script, NULL, NULL, 1, &exception);
 JSStringRelease(script);
 if (exception) { *error = dub_string(ctx, exception); return NULL; }
 return dub_string(ctx, result);
}
#else
#include <jsc/jsc.h>
typedef JSCContext *dub_context;
static dub_context dub_create(void) { return jsc_context_new(); }
static void dub_release(dub_context ctx) { g_object_unref(ctx); }
static char *dub_eval(dub_context ctx, const char *source, char **error) {
 JSCValue *value = jsc_context_evaluate(ctx, source, -1);
 JSCException *exception = jsc_context_get_exception(ctx);
 char *result = NULL;
 if (exception) {
  char *message = jsc_exception_to_string(exception);
  *error = strdup(message);
  g_free(message);
  jsc_context_clear_exception(ctx);
 } else {
  char *text = jsc_value_to_string(value);
  result = strdup(text);
  g_free(text);
 }
 if (value) g_object_unref(value);
 return result;
}
#endif
*/
import "C"

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
	"unsafe"
)

type jsContext struct{ handle C.dub_context }

func newJSContext() *jsContext { return &jsContext{handle: C.dub_create()} }

func (ctx *jsContext) close() {
	if ctx.handle != nil {
		C.dub_release(ctx.handle)
		ctx.handle = nil
	}
}

func (ctx *jsContext) eval(source string) (string, error) {
	if ctx.handle == nil {
		return "", errors.New("JavaScriptCore context is closed")
	}
	script := C.CString(source)
	defer C.free(unsafe.Pointer(script))
	var exception *C.char
	result := C.dub_eval(ctx.handle, script, &exception)
	if exception != nil {
		defer C.free(unsafe.Pointer(exception))
		return "", errors.New(C.GoString(exception))
	}
	if result == nil {
		return "", errors.New("JavaScriptCore returned no result")
	}
	defer C.free(unsafe.Pointer(result))
	return C.GoString(result), nil
}

func (ctx *jsContext) run(expression string, timeout time.Duration) error {
	_, err := ctx.eval(`globalThis.testResult = {done:false,error:""};
 Promise.resolve().then(()=>` + expression + `).then(
 ()=>{testResult.done=true;},
 error=>{testResult.error=String(error)+"\n"+String(error?.stack || "");testResult.done=true;});`)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		state, err := ctx.eval("JSON.stringify(testResult)")
		if err != nil {
			return err
		}
		var result struct {
			Done  bool
			Error string
		}
		if err := json.Unmarshal([]byte(state), &result); err != nil {
			return err
		}
		if result.Done {
			if result.Error != "" {
				return errors.New(result.Error)
			}
			return nil
		}
		time.Sleep(time.Millisecond)
	}
	return fmt.Errorf("JavaScript test timed out after %s", timeout)
}
