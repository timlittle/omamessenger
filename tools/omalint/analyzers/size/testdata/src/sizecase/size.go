package sizecase

func threshold(a, b, c, d, e int) {}

func over(a, b, c, d, e, f int) {} // want "size: function has 6 parameters; maximum is 5"

//omalint:ignore size fixture proves the suppression path
func suppressed(a, b, c, d, e, f int) {}

//omalint:ignore size // want "suppression requires"
func missingReason(a, b, c, d, e, f int) {} // want "size: function has 6 parameters; maximum is 5"
