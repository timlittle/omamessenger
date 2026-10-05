package crossfile

func reportedInOtherFile(a, b, c, d, e, f int) {} // want "size: function has 6 parameters; maximum is 5"

func unnamedParams(int, int, int, int, int, int) {} // want "size: function has 6 parameters; maximum is 5"
