package main

import (
	"testing"

	examplev1 "github.com/tetriz-ai/protoauthz/gen/protoauthz/example/v1"
	"github.com/tetriz-ai/protoauthz/protoauthztest"
)

// TestEveryRPCHasAnAuthLevel is the test to put in your own service's suite.
// It fails naming each RPC with no usable auth_level. Pointed at the
// example's draft notes service, which leaves DeleteNote without a level, it
// fails with:
//
//	/protoauthz.example.draft.v1.NoteService/DeleteNote has no usable (protoauthz.v1.auth_level) option
func TestEveryRPCHasAnAuthLevel(t *testing.T) {
	protoauthztest.RequireAuthLevels(t, examplev1.File_protoauthz_example_v1_notes_proto)
}
