# 0.2.0 / 2026-10-25

- add: decode bracket keys like `types[]=a&types[]=b` and `created_at[gte]=…` in queries and form bodies, as sent by the Anthropic SDKs, Rails and jQuery
- add: decode repeated `tags=a&tags=b`, or a single `tags=a`, into slice fields
- **BREAKING:** a query key now conflicts with a body field that shares its top-level name (e.g. `?tags.0=a` with `{"tags":[…]}`) and returns 400; previously the body silently won
- **BREAKING:** malformed bracket keys like `a[` or `a[][b]` return 400

# 0.1.5 / 2026-10-07

- switch to `encoding/json/v2` to support nil maps and arrays defaulting to `{}` and `[]`

# 0.1.4 / 2026-10-06

- validate outgoing structs as well with the `validate` tag or a `Validate() koi.Invalid` method; invalid response bodies return 500

# 0.1.3 / 2026-10-02

- send json to browsers if there's no view

# 0.1.2 / 2026-09-29

- limit body size, map 413/415, and fix validation and duplicate field names

# 0.1.1 / 2026-09-29

- add support for multipart uploads
- add a few tests
- skip reading and validating if the input is empty
- test that we support form maps

# 0.1.0 / 2026-09-18

- Large refactor of koi implementation

# 0.0.2 / 2025-08-31

- add custom validator implementation

# 0.0.1 / 2025-08-31

- change up the API

# 0.0.0 / 2019-12-20

- fix go mod and tests
- add failing test
- add failing test
- support content negotiation and better error codes
- switch Wrap to JSON to prep for changes in #5
- minor prepworkl
- update readme for v2
- full example in readme
- simplify koi package
- update readme
- remove validation, support unmarshalable types
- unroll
- one file
- add fish
- write function
- custom write function
- unwrap the results and errors. add tests for 400 errors
- add an example to the readme
- add middleware support
- add more commented out tests
- update readme
- add another example
- remove -v
- add a makefile
- usable version complete
- bug fixes, an example and a readme
- Initial commit
