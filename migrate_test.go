package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

func TestFixConfigServerParams(t *testing.T) {
	for i, test := range []struct {
		in  string
		out string
	}{
		{ // noop if everything is compatible
			in:  `{ "type": "foo", "number": 36, "enabled": true }`,
			out: `{ "type": "foo", "number": 36, "enabled": true }`,
		},
		{ // remove git.repos
			in:  `{ "git": { "repos": [], "foo": "bar" } }`,
			out: `{ "git": { "foo": "bar" } }`,
		},
		{ // fix composite git - add type
			in:  `{ "composite": [ { "git": {} } ] }`,
			out: `{ "composite": [ { "type": "git" } ] }`,
		},
		{ // fix composite vault, keeping any remaining key/value pairs
			in:  `{ "composite": [ { "vault": { "foo": 42 } } ] }`,
			out: `{ "composite": [ { "type": "vault", "foo": 42 } ] }`,
		},
		{ // multiple composites
			in:  `{ "composite": [ { "git": { "foo": "bar" } }, { "vault": { "baz": 42 } }  ] }`,
			out: `{ "composite": [ { "type": "git", "foo": "bar" }, { "type": "vault", "baz": 42 } ] }`,
		},
	} {
		t.Run(fmt.Sprintf("test %d", i), func(t *testing.T) {
			inmap := make(map[string]interface{})
			if err := json.Unmarshal([]byte(test.in), &inmap); err != nil {
				t.Fatal(err)
			}
			outmap := make(map[string]interface{})
			if err := json.Unmarshal([]byte(test.out), &outmap); err != nil {
				t.Fatal(err)
			}

			fixConfigServerParams(inmap)

			if !reflect.DeepEqual(inmap, outmap) {
				actual, _ := json.Marshal(inmap)
				t.Fatalf("JSON doesn't match\nwant: %v\ngot: %v\n", test.out, string(actual))
			}
		})
	}
}
