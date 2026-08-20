package agent

import "os"

// readFile returns the file's bytes or nil on error. Used by RunAgent to
// load the controller-assigned result.json and declared artifacts after the
// runtime reports process completion.
func readFile(path string) []byte {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return data
}

// readArtifacts reads each declared-output path and returns a name->bytes
// map. Missing files are simply absent from the map; the completion check
// gates on presence, not on the map containing every declared name.
func readArtifacts(paths map[string]string) map[string][]byte {
	out := make(map[string][]byte, len(paths))
	for name, path := range paths {
		if path == "" {
			continue
		}
		if body := readFile(path); len(body) > 0 {
			out[name] = body
		}
	}
	return out
}