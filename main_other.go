//go:build !linux

package main

func platformInit(string)    {}
func platformCleanup(string) {}
