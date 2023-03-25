package v1

const (
	ResourceTartVMs = "org.cirruslabs.tart-vms"
)

type Resources map[string]uint64

func (resources Resources) Copy() Resources {
	result := make(Resources)

	for key, value := range resources {
		result[key] = value
	}

	return result
}

func (resources Resources) Add(other Resources) {
	for otherKey, otherValue := range other {
		resources[otherKey] += otherValue
	}
}

func (resources Resources) Added(other Resources) Resources {
	result := resources.Copy()

	for otherKey, otherValue := range other {
		result[otherKey] += otherValue
	}

	return result
}

func (resources Resources) Subtract(other Resources) {
	for otherKey, otherValue := range other {
		resources[otherKey] -= otherValue
	}
}

func (resources Resources) Subtracted(other Resources) Resources {
	result := resources.Copy()

	for otherKey, otherValue := range other {
		resources[otherKey] -= otherValue
	}

	return result
}

func (resources Resources) CanFit(other Resources) bool {
	for otherKey, otherValue := range other {
		if otherValue > resources[otherKey] {
			return false
		}
	}

	return true
}
