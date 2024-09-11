package main

import "fmt"

// Panics in a goroutine crash the whole server.
// These wrappers minimize the boilerplate to protect against this.

// function takes no parameters
func safeGo(fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("Recovered from panic: %v\n", r)
			}
		}()
		fn()
	}()
}

// function takes 1 parameter
func safeGo1[T any](fn func(T)) func(T) {
	return func(arg T) {
		safeGo(func() {
			fn(arg)
		})
	}
}

// function takes 2 parameters
func safeGo2[T1, T2 any](fn func(T1, T2)) func(T1, T2) {
	return func(arg1 T1, arg2 T2) {
		safeGo(func() {
			fn(arg1, arg2)
		})
	}
}

// function takes 3 parameters
func safeGo3[T1, T2, T3 any](fn func(T1, T2, T3)) func(T1, T2, T3) {
	return func(arg1 T1, arg2 T2, arg3 T3) {
		safeGo(func() {
			fn(arg1, arg2, arg3)
		})
	}
}

// function takes 4 parameters
func safeGo4[T1, T2, T3, T4 any](fn func(T1, T2, T3, T4)) func(T1, T2, T3, T4) {
	return func(arg1 T1, arg2 T2, arg3 T3, arg4 T4) {
		safeGo(func() {
			fn(arg1, arg2, arg3, arg4)
		})
	}
}
