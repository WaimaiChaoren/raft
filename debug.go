package raft

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"
)

//------------------------------------------------------------------------------
//
// Variables
//
//------------------------------------------------------------------------------

const (
	Debug = 1
	Trace = 2
)

var logLevel int = 0
var logger *log.Logger

func init() {
	logger = log.New(os.Stdout, "[raft]", log.Lmicroseconds)
}

//------------------------------------------------------------------------------
//
// Functions
//
//------------------------------------------------------------------------------

func LogLevel() int {
	return logLevel
}

func SetLogLevel(level int) {
	logLevel = level
}

//--------------------------------------
// Warnings
//--------------------------------------

// Prints to the standard logger. Arguments are handled in the manner of
// fmt.Print.
func warn(args ...interface{}) {
	pc, _, line, _ := runtime.Caller(1)
    	function := runtime.FuncForPC(pc)
    	funcname := function.Name()
    	msg := fmt.Sprintf(generateFmtStr(len(args)), args...)
    	log.Printf("[%s : %d] %s", funcname, line, msg)
}

// Prints to the standard logger. Arguments are handled in the manner of
// fmt.Printf.
func warnf(format string, args ...interface{}) {
	pc, _, line, _ := runtime.Caller(1)
    	function := runtime.FuncForPC(pc)
    	funcname := function.Name()
    	msg := fmt.Sprintf(format, args...)
    	log.Printf("[%s : %d] %s", funcname, line, msg)
}

// Prints to the standard logger. Arguments are handled in the manner of
// fmt.Println.
func warnln(args ...interface{}) {
	pc, _, line, _ := runtime.Caller(1)
    	function := runtime.FuncForPC(pc)
    	funcname := function.Name()
    	msg := fmt.Sprintf(generateFmtStr(len(args)), args...)
    	log.Printf("[%s : %d] %s", funcname, line, msg)
}

//--------------------------------------
// Basic debugging
//--------------------------------------

// Prints to the standard logger if debug mode is enabled. Arguments
// are handled in the manner of fmt.Print.
func debug(args ...interface{}) {
	if logLevel >= Debug {
		pc, _, line, _ := runtime.Caller(1)
    		function := runtime.FuncForPC(pc)
    		funcname := function.Name()
    		msg := fmt.Sprintf(generateFmtStr(len(args)), args...)
    		log.Printf("[%s : %d] %s", funcname, line, msg)
	}
}

// Prints to the standard logger if debug mode is enabled. Arguments
// are handled in the manner of fmt.Printf.
func debugf(format string, args ...interface{}) {
	if logLevel >= Debug {
		pc, _, line, _ := runtime.Caller(1)
    		function := runtime.FuncForPC(pc)
    		funcname := function.Name()
    		msg := fmt.Sprintf(format, args...)
    		log.Printf("[%s : %d] %s", funcname, line, msg)
	}
}

// Prints to the standard logger if debug mode is enabled. Arguments
// are handled in the manner of fmt.Println.
func debugln(args ...interface{}) {
	if logLevel >= Debug {
		pc, _, line, _ := runtime.Caller(1)
    		function := runtime.FuncForPC(pc)
    		funcname := function.Name()
    		msg := fmt.Sprintf(generateFmtStr(len(args)), args...)
    		log.Printf("[%s : %d] %s", funcname, line, msg)
	}
}

//--------------------------------------
// Trace-level debugging
//--------------------------------------

// Prints to the standard logger if trace debugging is enabled. Arguments
// are handled in the manner of fmt.Print.
func trace(args ...interface{}) {
	if logLevel >= Trace {
		pc, _, line, _ := runtime.Caller(1)
    		function := runtime.FuncForPC(pc)
    		funcname := function.Name()
    		msg := fmt.Sprintf(generateFmtStr(len(args)), args...)
    		log.Printf("[%s : %d] %s", funcname, line, msg)
	}
}


func tracef(format string, args ...interface{}) {
	if logLevel >= Trace {
    		pc, _, line, _ := runtime.Caller(1)
    		function := runtime.FuncForPC(pc)
    		funcname := function.Name()
    		msg := fmt.Sprintf(format, args...)
    		log.Printf("[%s : %d] %s", funcname, line, msg)
	}
}



func traceln(args ...interface{}) {
	if logLevel >= Trace {
    		pc, _, line, _ := runtime.Caller(1)
    		function := runtime.FuncForPC(pc)
    		funcname := function.Name()
    		msg := fmt.Sprintf(generateFmtStr(len(args)), args...)
    		log.Printf("[%s : %d] %s", funcname, line, msg)
	}
}


func generateFmtStr(n int) string {
        return strings.Repeat("%v ", n)
}


