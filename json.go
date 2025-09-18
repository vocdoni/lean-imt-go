package leanimt

import (
	"encoding/json"
	"errors"
)

// Export encodes the internal matrix as JSON.
// For *big.Int values, this results in JSON strings (via TextMarshaler),
// matching the TS behavior that stringifies bigints.
func (t *LeanIMT[N]) Export() (string, error) {
	b, err := json.Marshal(t.nodes)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Import parses a JSON-encoded nodes matrix and returns a new tree.
// If mapFn is provided, every JSON scalar value that is encoded as a string
// will be passed through mapFn to build values of type N.
// If mapFn is nil, Import attempts to unmarshal directly into [][]N.
func Import[N any](hash Hasher[N], nodesJSON string, eq Equal[N], mapFn func(string) (N, error)) (*LeanIMT[N], error) {
	if hash == nil {
		return nil, errors.New("parameter 'hash' is not defined")
	}
	if nodesJSON == "" {
		return nil, errors.New("parameter 'nodes' is not defined")
	}

	tree := &LeanIMT[N]{
		nodes: [][]N{ /* replaced below */ },
		hash:  hash,
		eq:    eq,
	}

	// If no map needed, try direct unmarshal into [][]N.
	if mapFn == nil {
		var nodes [][]N
		if err := json.Unmarshal([]byte(nodesJSON), &nodes); err != nil {
			return nil, err
		}
		tree.nodes = nodes
		return tree, nil
	}

	// Otherwise, unmarshal into [][]any, convert strings via mapFn (only).
	var raw [][]any
	if err := json.Unmarshal([]byte(nodesJSON), &raw); err != nil {
		return nil, err
	}

	nodes := make([][]N, len(raw))
	for i := range raw {
		nodes[i] = make([]N, len(raw[i]))
		for j, v := range raw[i] {
			s, ok := v.(string)
			if !ok {
				// If it's not a string, try marshaling it back to JSON then feed mapFn on the string form.
				b, _ := json.Marshal(v)
				s = string(b)
			}
			val, err := mapFn(s)
			if err != nil {
				return nil, err
			}
			nodes[i][j] = val
		}
	}
	tree.nodes = nodes
	return tree, nil
}

