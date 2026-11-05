package cli

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"

	"github.com/srliao/tt/internal/tag"
)

func newTagCmd(s *session) *cobra.Command {
	return group("tag", "List and create tags",
		newTagListCmd(s),
		newTagCreateCmd(s),
	)
}

const tagShape = `{id, name, color_hue, created_at}`

func newTagListCmd(s *session) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List every tag with its task count",
		Long: `List every tag. Task commands only accept tags listed here.

Output: {tags: [{id, name, color_hue, created_at, count}]}`,
		Example: `  tt-cli tag list`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out, err := list[tag.TagWithCount](cmd.Context(), s, http.MethodGet, "/tags", url.Values{"counts": {"1"}}, nil)
			if err != nil {
				return err
			}
			return s.out(map[string]any{"tags": out})
		},
	}
}

func newTagCreateCmd(s *session) *cobra.Command {
	return &cobra.Command{
		Use:   "create <name>",
		Short: "Create a tag",
		Long: `Create a tag. Names are trimmed and lowercased; names starting with @ are
reserved. Creating an existing tag returns it unchanged.

Output: ` + tagShape,
		Example: `  tt-cli tag create errand`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var out tag.Tag
			if err := s.do(cmd.Context(), http.MethodPost, "/tags", nil, map[string]string{"name": args[0]}, &out); err != nil {
				return err
			}
			return s.out(out)
		},
	}
}
