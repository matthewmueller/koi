# Koi

WIP

## As a router

```go
api := koi.New()

type Input struct {
  Hello string `json:"hello,omitempty" validate:"required"`
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

## As Middleware

```go
router := httprouter.New()
handle := router.HandlerFunc
koi := koi.New()

type Input struct {
  Hello string `json:"hello,omitempty" validate:"required"`
}

type Output struct {
  Goodbye string `json:"goodbye,omitempty"`
}

handle("POST", "/", koi.Wrap(func(p struct { User string },v Input) (o Output, e error) {
  o.Goodbye = "adios!"
  return o, nil
}))

http.ListenAndServe(":3000", router)
```

## License

MIT