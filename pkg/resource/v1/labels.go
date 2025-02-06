package v1

type Labels map[string]string

func (labels Labels) Contains(other Labels) bool {
	for label, value := range other {
		if labels[label] != value {
			return false
		}
	}

	return true
}
