# env
A Go module for easily reading environment variables into memory

```go
type TokenConfig struct {
  Token         string        `env:"APP_TOKEN"`
  TokenLifetime time.Duration `env:"APP_TOKEN_LIFETIME" env-default="2h"`
  TokenMax      uint64        `env:"APP_TOKEN_MAX" env-default="32"`
}

tokenConfig, err := env.Get[TokenConfig]()
```

### Features
- Parses via reflection & struct tags
- Parses all basic data types (primitives, structs, arrays, slices, embedded/anonymous structs)
- Handles embedded structs and struct fields
- Caches parsed object (use `env.Get[T]()`)
- Supports custom unmarshalling & parsing functions
    - `env.Unmarshaller`
    - `encoding.TextUnmarshaler`
    - `env.RegisterParser[T](fn env.Parser)`
- Supports multiple environment variables per field
- Supports default values
- Supports custom delimiters for arrays & slices 
- Supports post-validation logic 
    - `env.Validator`
- Supports nested variable names
    ```go
    type Connection struct {
        User string `env:"USER"`
        Pass string `env:"PASS"`
        Host string `env:"HOST"`
        Port uint16 `env:"PORT"`
    }
    type Config struct {
        // all variables in field are prefixed with this, so DB_USER_HOST
        UserDatabase Connection `env:"DB_USER_"`
        MainDatabase Connection `env:"DB_MAIN_"`
    }
    ```
- Supports unnesting variable names `env:"^DB_USER"`