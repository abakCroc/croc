//go:build capi

package main

/*
#cgo LDFLAGS: -landroid -llog
#include <jni.h>
#include <stdlib.h>

static const char* jstringToUTF(JNIEnv *env, jstring s) {
    return (*env)->GetStringUTFChars(env, s, NULL);
}
static void releaseUTF(JNIEnv *env, jstring s, const char* utf) {
    (*env)->ReleaseStringUTFChars(env, s, utf);
}
static jstring getStringArrayElement(JNIEnv *env, jobjectArray arr, int i) {
    return (*env)->GetObjectArrayElement(env, arr, i);
}
*/
import "C"

import (
	"encoding/json"
	"log"
	"os"
	"sync"

	"github.com/schollz/croc/v10/src/cli"
	"github.com/schollz/croc/v10/src/utils"
)

var (
	runWaitGroup sync.WaitGroup
	runResult    error
	pipeReader   *os.File
	origStderr   *os.File
	cancelCh     chan struct{}
)

type crocConfig struct {
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	WorkDir string            `json:"workDir"`
}

//export Java_com_dking_crocapp_croc_CrocNative_crocStart
func Java_com_dking_crocapp_croc_CrocNative_crocStart(
	env *C.JNIEnv, obj C.jobject, configJson C.jstring,
) C.jint {
	cfgStr := C.GoString(C.jstringToUTF(env, configJson))
	C.releaseUTF(env, configJson, C.jstringToUTF(env, configJson))

	var cfg crocConfig
	if err := json.Unmarshal([]byte(cfgStr), &cfg); err != nil {
		log.Printf("capi: failed to parse config JSON: %v", err)
		return -1
	}

	os.Args = cfg.Args
	for k, v := range cfg.Env {
		os.Setenv(k, v)
	}
	if cfg.WorkDir != "" {
		if err := os.Chdir(cfg.WorkDir); err != nil {
			log.Printf("capi: chdir failed: %v", err)
			return -1
		}
	}

	r, w, err := os.Pipe()
	if err != nil {
		log.Printf("capi: pipe failed: %v", err)
		return -1
	}

	origStderr = os.Stderr
	os.Stderr = w
	pipeReader = r

	cancelCh = make(chan struct{})

	runWaitGroup.Add(1)
	go func() {
		defer runWaitGroup.Done()
		runResult = cli.Run()
		w.Close()
	}()

	return C.jint(r.Fd())
}

//export Java_com_dking_crocapp_croc_CrocNative_crocWait
func Java_com_dking_crocapp_croc_CrocNative_crocWait(
	env *C.JNIEnv, obj C.jobject,
) C.jint {
	runWaitGroup.Wait()

	if origStderr != nil {
		os.Stderr = origStderr
	}
	if pipeReader != nil {
		pipeReader.Close()
		pipeReader = nil
	}

	utils.RemoveMarkedFiles()

	if runResult != nil {
		log.Printf("capi: cli.Run returned error: %v", runResult)
		return 1
	}
	return 0
}

//export Java_com_dking_crocapp_croc_CrocNative_crocCancel
func Java_com_dking_crocapp_croc_CrocNative_crocCancel(
	env *C.JNIEnv, obj C.jobject,
) {
	select {
	case <-cancelCh:
	default:
		close(cancelCh)
	}
}

func main() {}
