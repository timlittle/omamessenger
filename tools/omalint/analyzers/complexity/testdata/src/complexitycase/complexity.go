package complexitycase

func ifLimit() {
	if true {
	}
	if true {
	}
	if true {
	}
	if true {
	}
	if true {
	}
	if true {
	}
	if true {
	}
	if true {
	}
	if true {
	}
}

func ifOver() { // want "complexity: ifOver has complexity 11"
	if true {
	}
	if true {
	}
	if true {
	}
	if true {
	}
	if true {
	}
	if true {
	}
	if true {
	}
	if true {
	}
	if true {
	}
	if true {
	}
}

func forLimit() {
	for i := 0; i < 1; i++ {
	}
	for i := 0; i < 1; i++ {
	}
	for i := 0; i < 1; i++ {
	}
	for i := 0; i < 1; i++ {
	}
	for i := 0; i < 1; i++ {
	}
	for i := 0; i < 1; i++ {
	}
	for i := 0; i < 1; i++ {
	}
	for i := 0; i < 1; i++ {
	}
	for i := 0; i < 1; i++ {
	}
}

func forOver() { // want "complexity: forOver has complexity 11"
	for i := 0; i < 1; i++ {
	}
	for i := 0; i < 1; i++ {
	}
	for i := 0; i < 1; i++ {
	}
	for i := 0; i < 1; i++ {
	}
	for i := 0; i < 1; i++ {
	}
	for i := 0; i < 1; i++ {
	}
	for i := 0; i < 1; i++ {
	}
	for i := 0; i < 1; i++ {
	}
	for i := 0; i < 1; i++ {
	}
	for i := 0; i < 1; i++ {
	}
}

func rangeLimit() {
	xs := []int{}
	for range xs {
	}
	for range xs {
	}
	for range xs {
	}
	for range xs {
	}
	for range xs {
	}
	for range xs {
	}
	for range xs {
	}
	for range xs {
	}
	for range xs {
	}
}

func rangeOver() { // want "complexity: rangeOver has complexity 11"
	xs := []int{}
	for range xs {
	}
	for range xs {
	}
	for range xs {
	}
	for range xs {
	}
	for range xs {
	}
	for range xs {
	}
	for range xs {
	}
	for range xs {
	}
	for range xs {
	}
	for range xs {
	}
}

func caseLimit(v int) {
	switch v {
	case 0:
	case 1:
	case 2:
	case 3:
	case 4:
	case 5:
	case 6:
	case 7:
	case 8:
	default:
	}
}

func caseOver(v int) { // want "complexity: caseOver has complexity 11"
	switch v {
	case 0:
	case 1:
	case 2:
	case 3:
	case 4:
	case 5:
	case 6:
	case 7:
	case 8:
	case 9:
	default:
	}
}

func commLimit(ch chan int) {
	select {
	case <-ch:
	case <-ch:
	case <-ch:
	case <-ch:
	case <-ch:
	case <-ch:
	case <-ch:
	case <-ch:
	case <-ch:
	}
}

func commOver(ch chan int) { // want "complexity: commOver has complexity 11"
	select {
	case <-ch:
	case <-ch:
	case <-ch:
	case <-ch:
	case <-ch:
	case <-ch:
	case <-ch:
	case <-ch:
	case <-ch:
	case <-ch:
	}
}

func andLimit() bool {
	return true && true && true && true && true && true && true && true && true
}

func andOver() bool { // want "complexity: andOver has complexity 11"
	return true && true && true && true && true && true && true && true && true && true && true
}

func orLimit() bool {
	return true || true || true || true || true || true || true || true || true
}

func orOver() bool { // want "complexity: orOver has complexity 11"
	return true || true || true || true || true || true || true || true || true || true || true
}

func literalLimit() {
	_ = func() {
		if true {
		}
		if true {
		}
		if true {
		}
		if true {
		}
		if true {
		}
		if true {
		}
		if true {
		}
		if true {
		}
		if true {
		}
	}
}

func literalOver() {
	_ = func() { // want "complexity: function literal has complexity 11"
		if true {
		}
		if true {
		}
		if true {
		}
		if true {
		}
		if true {
		}
		if true {
		}
		if true {
		}
		if true {
		}
		if true {
		}
		if true {
		}
	}
}

func depthLimit() {
	if true {
		if true {
			if true {
				if true {
				}
			}
		}
	}
}

func depthOver() { // want "complexity: depthOver has nesting depth 5"
	if true {
		if true {
			if true {
				if true {
					if true {
					}
				}
			}
		}
	}
}

//omalint:ignore complexity fixture verifies a documented suppression
func suppressed() {
	if true {
	}
	if true {
	}
	if true {
	}
	if true {
	}
	if true {
	}
	if true {
	}
	if true {
	}
	if true {
	}
	if true {
	}
	if true {
	}
}

//omalint:ignore complexity // want "suppression requires"
func missingReason() { // want "complexity: missingReason has complexity 11"
	if true {
	}
	if true {
	}
	if true {
	}
	if true {
	}
	if true {
	}
	if true {
	}
	if true {
	}
	if true {
	}
	if true {
	}
	if true {
	}
}
