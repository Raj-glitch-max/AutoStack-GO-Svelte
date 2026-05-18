package workers

// HasCapability returns true when the worker has the given capability.
func HasCapability(w Worker, cap string) bool {
	for _, c := range w.Capabilities {
		if c == cap {
			return true
		}
	}
	return false
}

// CapabilitiesMatch returns true when the worker has ALL required capabilities.
// An empty required list always matches.
func CapabilitiesMatch(w Worker, required []string) bool {
	for _, req := range required {
		if !HasCapability(w, req) {
			return false
		}
	}
	return true
}

// FilterByCapabilities returns workers that satisfy all required capabilities.
func FilterByCapabilities(workers []Worker, required []string) []Worker {
	if len(required) == 0 {
		return workers
	}
	var out []Worker
	for _, w := range workers {
		if CapabilitiesMatch(w, required) {
			out = append(out, w)
		}
	}
	return out
}
