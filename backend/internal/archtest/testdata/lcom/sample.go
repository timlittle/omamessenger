package synthetic

type Sample struct {
	Shared   int
	Separate int
}

func (s *Sample) first() {
	_ = s.Shared
	s.second()
}

func (s *Sample) second() {
	_ = s.Shared
}

func (s *Sample) unrelated() {
	_ = s.Separate
}
