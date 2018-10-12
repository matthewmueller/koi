# Koi 🐠

[![](https://godoc.org/github.com/matthewmueller/koi?status.svg)](http://godoc.org/github.com/matthewmueller/koi)

Simple router and middleware.

## As a router

```go
api := koi.New()

type Input struct {
  Hello string `json:"hello,omitempty"`
}

type Output struct {
  Goodbye string `json:"goodbye,omitempty"`
}

api.Post("/:first/:last", func(r *koi.Request, v Input) (o Output, e error) {
  o.Goodbye = fmt.Sprintf("adios %s %s!", r.Param("first"), r.Param("last"))
  return o, nil
})

http.ListenAndServe(":3000", api)
```

## Router with middleware

```go
api := koi.New()

type Input struct {
  Hello string `json:"hello,omitempty"`
}

type Output struct {
  Goodbye string `json:"goodbye,omitempty"`
}

middleware := func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
  fmt.Println("a ->")
  next(w, r)
  fmt.Println("a <-")
}

api.Post("/:first/:last", middleware, func(r *koi.Request, v Input) (o Output, e error) {
  o.Goodbye = fmt.Sprintf("adios %s %s!", r.Param("first"), r.Param("last"))
  return o, nil
})

http.ListenAndServe(":3000", api)
```

## With request & response

```go
api := koi.New()

type Input struct {
  Hello string `json:"hello,omitempty"`
}

type Output struct {
  Goodbye string `json:"goodbye,omitempty"`
}

api.Post("/", func(w http.ResponseWriter, r *http.Request, v Input) (o Output, e error) {
  o.Goodbye = "adios!"
  return o, nil
})

http.ListenAndServe(":3000", api)
```

## As Middleware

```go
router := httprouter.New()
handle := router.HandlerFunc
koi := koi.New()

type Input struct {
  Hello string `json:"hello,omitempty"`
}

type Output struct {
  Goodbye string `json:"goodbye,omitempty"`
}

handle("POST", "/", koi.Wrap(func(v Input) (o Output, e error) {
  o.Goodbye = "adios!"
  return o, nil
}))

http.ListenAndServe(":3000", router)
```

## License

MIT