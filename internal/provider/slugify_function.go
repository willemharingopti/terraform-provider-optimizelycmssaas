package provider

import (
	"context"
	"strings"
	"unicode"

	"github.com/hashicorp/terraform-plugin-framework/function"
	"golang.org/x/text/unicode/norm"
)

var _ function.Function = &slugifyFunction{}

func NewSlugifyFunction() function.Function { return &slugifyFunction{} }

type slugifyFunction struct{}

func (f *slugifyFunction) Metadata(_ context.Context, _ function.MetadataRequest, resp *function.MetadataResponse) {
	resp.Name = "slugify"
}

func (f *slugifyFunction) Definition(_ context.Context, _ function.DefinitionRequest, resp *function.DefinitionResponse) {
	resp.Definition = function.Definition{
		Summary: "Turns a name into a key",
		Description: "Lowercases the text, strips accents (\"Café\" becomes \"cafe\") and replaces every run of other characters " +
			"with a single underscore, with none at the start or end. A leading digit gets an underscore in front. " +
			"For example, \"Main site\" becomes \"main_site\". The result is valid as a CMS key, which allows " +
			"only letters, digits and underscores and cannot start with a digit.",
		Parameters: []function.Parameter{
			function.StringParameter{Name: "text", Description: "The text to slugify."},
		},
		Return: function.StringReturn{},
	}
}

func (f *slugifyFunction) Run(ctx context.Context, req function.RunRequest, resp *function.RunResponse) {
	var text string
	if resp.Error = function.ConcatFuncErrors(req.Arguments.Get(ctx, &text)); resp.Error != nil {
		return
	}
	resp.Error = function.ConcatFuncErrors(resp.Result.Set(ctx, slugify(text)))
}

func slugify(s string) string {
	var b strings.Builder
	pendingSep := false
	for _, r := range norm.NFD.String(s) {
		switch {
		case unicode.Is(unicode.Mn, r): // combining accent left over from decomposition
		case r < unicode.MaxASCII && (unicode.IsLower(r) || unicode.IsUpper(r) || unicode.IsDigit(r)):
			if pendingSep && b.Len() > 0 {
				b.WriteByte('_')
			}
			pendingSep = false
			if b.Len() == 0 && unicode.IsDigit(r) {
				b.WriteByte('_')
			}
			b.WriteRune(unicode.ToLower(r))
		default:
			pendingSep = true
		}
	}
	return b.String()
}
