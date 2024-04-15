package utils

import "log"

// preventing application from crashing abruptly. use defer PanicRecover() on top of the codes that may cause panic
func PanicRecover() {
	if r := recover(); r != nil {
		log.Println("Recovered from panic: ", r)
	}
}
