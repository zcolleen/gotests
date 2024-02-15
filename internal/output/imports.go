package output

// we do not need support for aliases in import for now.
var importsMap = map[string][]string{
	"testify":  {"github.com/stretchr/testify/assert"},
	"minimock": {"github.com/stretchr/testify/assert", "github.com/gojuno/minimock/v3"},
}
