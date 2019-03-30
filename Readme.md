# Koi 🐠

[![](https://godoc.org/github.com/matthewmueller/koi?status.svg)](http://godoc.org/github.com/matthewmueller/koi)

Simple router and middleware.

## Example

```go
package main

import (
	"fmt"
	"net/http"

	"github.com/bmizerany/pat"
	"github.com/matthewmueller/koi"
	"github.com/rs/cors"
)

func main() {
	crossorigin := cors.AllowAll().Handler
	k := koi.New(crossorigin)
	router := pat.New()

	type Input struct {
		Hello string `json:"hello,omitempty"`
	}

	type Output struct {
		Goodbye string `json:"goodbye,omitempty"`
	}

	router.Post("/:first/:last", k.Wrap(func(r *http.Request, i Input) (o Output, err error) {
		query := r.URL.Query()
		o.Goodbye = fmt.Sprintf(`adios %s %s`, query.Get(":first"), query.Get(":last"))
		return o, nil
	}))

	http.ListenAndServe(":3000", router)
}
```

## License

MIT
