// Command find-unannotated-rpcs lists the RPCs that have no usable auth_level
// in two versions of a notes service: a draft with one RPC left bare, and the
// finished service.
package main

import (
	"fmt"

	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/tetriz-ai/protoauthz"
	draftv1 "github.com/tetriz-ai/protoauthz/gen/protoauthz/example/draft/v1"
	examplev1 "github.com/tetriz-ai/protoauthz/gen/protoauthz/example/v1"
)

func main() {
	for _, file := range []protoreflect.FileDescriptor{
		draftv1.File_protoauthz_example_draft_v1_notes_proto,
		examplev1.File_protoauthz_example_v1_notes_proto,
	} {
		missing := protoauthz.MissingAuthLevels(file)
		fmt.Printf("%s: %d missing %v\n", file.Path(), len(missing), missing)
	}
}
