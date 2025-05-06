package env

import (
	"encoding"
	"errors"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// An unmarshaller of an environment value given the current unmarshalling state.
type Unmarshaller interface {
	UnmarshalEnv(state UnmarshalState) error
}

// A validator that's ran after successful parsing. This cannot be used in
// conjuction with env.Unmarshaller or encoding.TextUnmarshaler.
type Validator interface {
	ValidateEnv(state UnmarshalState) error
}

// A custom parser for a given type.
type Parser func(state UnmarshalState) (any, error)

var (
	cacheLock sync.Mutex
	cache     map[reflect.Type]any
	parsers   map[reflect.Type]Parser
	kindBits  map[reflect.Kind]int

	// The struct tag which can store the environment variable name(s)
	// Skip can be used to skip a field. When multiple properties are defined,
	// they are examined one at a time until they find a specified value.
	TagEnv = "env"

	// The struct tag which defines a default value.
	TagEnvDefault = "env-default"

	// The struct tag which defines a custom delimiter for a slice/array value.
	TagEnvDelim = "env-delim"

	// The struct tag which defines a custom required option.
	TagEnvRequired = "env-required"

	// The delimiter for multiple environment variable names in the TagEnv struct tag.
	EnvDelimiter = ","

	// The default delimiter for slice/array values.
	DefaultDelimiter = ","

	// The value in the TagEnv struct tag that causes a field to be skipped.
	Skip = "-"

	// The prefix which determines that an environment value is absolute and not relative
	AbsoluteName = "^"

	// A required value (marked required or a non-pointer) is missing from the environment.
	ErrRequired = errors.New("required")

	// A value is missing from input. It may be okay if it's not required.
	ErrMissing = errors.New("missing")
)

func init() {
	cache = make(map[reflect.Type]any)
	parsers = make(map[reflect.Type]Parser)
	kindBits = map[reflect.Kind]int{
		reflect.Int8:    8,
		reflect.Int16:   16,
		reflect.Int32:   32,
		reflect.Int64:   64,
		reflect.Int:     64,
		reflect.Uint8:   8,
		reflect.Uint16:  16,
		reflect.Uint32:  32,
		reflect.Uint64:  64,
		reflect.Uint:    64,
		reflect.Float32: 32,
		reflect.Float64: 64,
	}

	// native parsers
	RegisterParser[time.Duration](func(state UnmarshalState) (any, error) {
		value, _ := state.Read()
		return time.ParseDuration(value)
	})
}

// Registers a custom parser for the given type.
func RegisterParser[T any](parser Parser) {
	key := reflect.TypeFor[T]()
	parsers[key] = parser
}

// Gets the cached or loads the environment variables for the given type.
func Get[T any]() (T, error) {
	key := reflect.TypeFor[T]()
	cached, exists := cache[key]
	if exists {
		return cached.(T), nil
	}

	cacheLock.Lock()
	defer cacheLock.Unlock()

	cached, exists = cache[key]
	if exists {
		return cached.(T), nil
	}

	cachedTyped, err := Load[T]()
	if err != nil {
		return cachedTyped, err
	}

	cache[key] = cachedTyped

	return cachedTyped, nil
}

// Gets the cached or loads the environment variables for the given type.
// If an error occurs a panic will be thrown.
func Must[T any]() T {
	gotten, err := Get[T]()
	if err != nil {
		panic(err)
	}
	return gotten
}

// Loads the type from environment variables.
func Load[T any]() (T, error) {
	var parsed T
	return parsed, Parse(&parsed)
}

// Loads the type from environment variables.
// If an error occurs a panic will be thrown.
func MustLoad[T any]() T {
	loaded, err := Load[T]()
	if err != nil {
		panic(err)
	}
	return loaded
}

// Loads the value (expected to be pointer) from environment variables.
func Parse(value any) error {
	var err error
	defer func() {
		if recovered := recover(); recovered != nil {
			if r, ok := recovered.(error); ok {
				err = r
			} else {
				err = fmt.Errorf("%v", recovered)
			}
		}
	}()

	rv := reflect.ValueOf(value)
	parseError := parse(rv, UnmarshalState{})
	if parseError != nil && !errors.Is(parseError, ErrMissing) {
		err = parseError
	}

	return err
}

func parse(rv reflect.Value, state UnmarshalState) error {
	if unmarshaller, ok := rv.Interface().(Unmarshaller); ok {
		return unmarshaller.UnmarshalEnv(state)
	}

	if unmarshaller, ok := rv.Interface().(encoding.TextUnmarshaler); ok {
		parsed, exists := state.Read()
		if !exists {
			return ErrMissing
		}
		return unmarshaller.UnmarshalText([]byte(parsed))
	}

	if parser, ok := parsers[rv.Type()]; ok {
		parsed, err := parser(state)
		if err != nil {
			return fmt.Errorf("error in custom parser for type %v: %w", rv.Type(), err)
		}
		rv.Set(reflect.ValueOf(parsed))
		return nil
	}

	// Complex types
	switch rv.Kind() {
	case reflect.Pointer:
		if rv.IsNil() {
			new := reflect.New(rv.Type().Elem())
			err := parse(new.Elem(), state)
			if err != nil {
				return err
			}
			rv.Set(new)
		} else {
			return parse(rv.Elem(), state)
		}
	case reflect.Array:
		text, exists := state.Read()
		if !exists {
			return ErrMissing
		}
		split, err := state.Split(text, rv.Len())
		if err != nil {
			return fmt.Errorf("error splitting: %w", err)
		}
		if len(split) != rv.Len() {
			return fmt.Errorf("cannot parse array from env, expected %d elements but got %d for %s", rv.Len(), len(split), state)
		}
		for i, s := range split {
			splitState := state
			splitState.read = &s
			splitState.readExists = true
			err := parse(rv.Index(i), splitState)
			if err != nil {
				return fmt.Errorf("at index %d: %w", i, err)
			}
		}
	case reflect.Slice:
		text, exists := state.Read()
		if !exists {
			return ErrMissing
		}
		if text == "" {
			return nil
		}
		split, err := state.Split(text, -1)
		if err != nil {
			return fmt.Errorf("error splitting: %w", err)
		}
		rv.Set(reflect.MakeSlice(rv.Type(), len(split), len(split)))
		for i, s := range split {
			splitState := state
			splitState.read = &s
			splitState.readExists = true
			err := parse(rv.Index(i), splitState)
			if err != nil {
				return fmt.Errorf("at index %d: %w", i, err)
			}
		}
	case reflect.Struct:
		valid := 0
		missing := 0
		var firstError error

		for i := range rv.NumField() {
			fieldStruct := rv.Type().Field(i)
			field := rv.Field(i)
			fieldState, skip := newFieldState(fieldStruct, state)
			if skip {
				continue
			}

			err := parse(field.Addr(), fieldState)

			if err != nil {
				isMissing := errors.Is(err, ErrMissing)
				isRequired := errors.Is(err, ErrRequired)
				if isMissing || isRequired {
					required, requiredErr := fieldState.Required(field.Kind() != reflect.Pointer)
					if requiredErr != nil {
						return fmt.Errorf("parsing %s of %s: %w", TagEnvRequired, fieldState, requiredErr)
					}
					if required {
						if isRequired {
							firstError = err
						} else {
							firstError = fmt.Errorf("%s: %w", fieldState, ErrRequired)
						}
					}
					missing++
				} else {
					return fmt.Errorf("%s: %w", fieldState, err)
				}
			} else {
				valid++
			}
		}
		if firstError != nil {
			return firstError
		}
		if valid == 0 && missing > 0 {
			return ErrMissing
		}

	case reflect.Chan, reflect.Complex128, reflect.Complex64, reflect.Func, reflect.Interface, reflect.Map, reflect.Invalid, reflect.Uintptr, reflect.UnsafePointer:
		return fmt.Errorf("kind %s not supported", rv.Kind())
	default:
		// For simple types, text should be an actual value.
		text, exists := state.Read()
		if !exists {
			return ErrMissing
		}

		// Simple types
		switch rv.Kind() {
		case reflect.String:
			rv.SetString(text)
		case reflect.Bool:
			parsed, err := strconv.ParseBool(text)
			if err != nil {
				return err
			}
			rv.SetBool(parsed)
		case reflect.Float32, reflect.Float64:
			parsed, err := strconv.ParseFloat(text, kindBits[rv.Kind()])
			if err != nil {
				return err
			}
			rv.SetFloat(parsed)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			parsed, err := strconv.ParseInt(text, 10, kindBits[rv.Kind()])
			if err != nil {
				return err
			}
			rv.SetInt(parsed)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			parsed, err := strconv.ParseUint(text, 10, kindBits[rv.Kind()])
			if err != nil {
				return err
			}
			rv.SetUint(parsed)
		}
	}

	if validator, ok := rv.Interface().(Validator); ok {
		return validator.ValidateEnv(state)
	}

	return nil
}

// The state of unmarshalling a value from the environment.
type UnmarshalState struct {
	Field     *reflect.StructField
	Variables []string

	read       *string
	readExists bool
}

// Creates a new UnmarshalState for the given struct field and parent state
func newFieldState(field reflect.StructField, parent UnmarshalState) (fieldState UnmarshalState, skip bool) {
	fieldState = UnmarshalState{
		Field: &field,
	}

	defaultVariable := field.Name
	if field.Anonymous {
		defaultVariable = ""
	}
	envs := fieldState.Envs(defaultVariable)
	if envs == nil {
		skip = true
		return
	}

	if len(parent.Variables) == 0 {
		fieldState.Variables = envs
	} else {
		for _, stateVar := range parent.Variables {
			for _, fieldVar := range envs {
				if strings.HasPrefix(fieldVar, AbsoluteName) {
					fieldState.Variables = append(fieldState.Variables, strings.TrimPrefix(fieldVar, AbsoluteName))
				} else {
					fieldState.Variables = append(fieldState.Variables, stateVar+fieldVar)
				}
			}
		}
	}

	return
}

// Reads the environment value defined by the variables in this state.
// Returns the whether the value or a default exists at all.
func (us *UnmarshalState) Read() (value string, exists bool) {
	if us.read != nil {
		return *us.read, us.readExists
	}
	for _, varName := range us.Variables {
		value, exists = os.LookupEnv(varName)
		if exists {
			break
		}
	}
	if !exists {
		value, exists = us.Default("")
	}
	us.read = &value
	us.readExists = exists
	return
}

// Returns the environment variable names for this state, EnvDelimiter delimited.
func (us UnmarshalState) String() string {
	return strings.Join(us.Variables, EnvDelimiter)
}

// Returns the partial environment variable names specified in the TagEnv struct tag.
func (us UnmarshalState) Envs(defaultValue string) []string {
	env, _ := us.Tag(TagEnv, defaultValue)
	if env == Skip {
		return nil
	}
	return strings.Split(env, EnvDelimiter)
}

// Returns the struct tag value for the given key, defaulting to a specific
// value if it's missing - and returns whether the tag exists.
func (us UnmarshalState) Tag(key string, missing string) (string, bool) {
	if us.Field == nil {
		return missing, false
	}
	value, exists := us.Field.Tag.Lookup(key)
	if !exists {
		return missing, false
	}
	return value, true
}

// Returns the default value specified on the struct tag if any exists.
func (us UnmarshalState) Default(otherwise string) (string, bool) {
	return us.Tag(TagEnvDefault, otherwise)
}

// Returns whether this value is required based on whether the type
// appears required and what the TagEnvRequired struct tag says.
func (us UnmarshalState) Required(appearsRequired bool) (bool, error) {
	defaultText := "false"
	if appearsRequired {
		defaultText = "true"
	}
	requiredText, exists := us.Tag(TagEnvRequired, defaultText)
	if !exists {
		return appearsRequired, nil
	}
	return strconv.ParseBool(requiredText)
}

// Returns a regular expression to split array/split values based on
// the env.TagEnvDelim struct tag and env.DefaultDelimiter.
func (us UnmarshalState) Delim() (*regexp.Regexp, error) {
	delimiter, _ := us.Tag(TagEnvDelim, DefaultDelimiter)
	return regexp.Compile(delimiter)
}

// Returns a split set of values based on the input string, max number of times,
// and the delimiter expression specified on the struct tag.
func (us UnmarshalState) Split(s string, times int) ([]string, error) {
	delim, err := us.Delim()
	if err != nil {
		return nil, err
	}
	return delim.Split(s, times), nil
}
