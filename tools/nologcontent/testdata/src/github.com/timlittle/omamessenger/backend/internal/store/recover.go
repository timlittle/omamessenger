package store

import (
	"fmt"
	"log"
)

// safeRecover logs only the recovered value's own type, the one
// allowed shape.
func safeRecover() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("panic: %T", r)
		}
	}()
}

// unsafeRecoverVerbose formats the recovered value with %v, which could
// format whatever the panic actually carried.
func unsafeRecoverVerbose() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("panic: %v", r) // want "nologcontent: a recovered panic value reached log.Printf formatted with anything but %T"
		}
	}()
}

// unsafeRecoverString formats the recovered value with %s.
func unsafeRecoverString() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("panic: %s", r) // want "nologcontent: a recovered panic value reached log.Printf formatted with anything but %T"
		}
	}()
}

// unsafeRecoverPrint passes the recovered value to a Print call with no
// format string at all, which still formats it with %v.
func unsafeRecoverPrint() {
	defer func() {
		if r := recover(); r != nil {
			log.Print(r) // want "nologcontent: a recovered panic value reached log.Print formatted with anything but %T"
			fmt.Print(r) // want "nologcontent: a recovered panic value reached fmt.Print formatted with anything but %T"
		}
	}()
}

// unsafeRecoverWrapped formats the recovered value through fmt.Errorf,
// which still only ever lets %T past safely, checked even though
// Errorf never writes anywhere itself.
func unsafeRecoverWrapped() {
	defer func() {
		if r := recover(); r != nil {
			log.Print(fmt.Errorf("panic: %v", r)) // want "nologcontent: a recovered panic value reached fmt.Errorf formatted with anything but %T"
		}
	}()
}
