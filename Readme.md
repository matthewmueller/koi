# Koi

WIP

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

handle("POST", "/", koi(func(v Input) (o Output, e error) {
  assert.Equal(t, v.Hello, "hola")
  o.Goodbye = "adios!"
  return o, nil
}))

http.ListenAndServe(":3000", router)
```

## License

MIT