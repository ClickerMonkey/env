package env_test

import (
	"encoding"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/clickermonkey/env"
	"github.com/stretchr/testify/assert"
)

type SimpleWithoutTags struct {
	WITHOUT_TAGS string
}

type SimpleWithTags struct {
	Text     string        `env:"SIMPLE_WT_TEXT"`
	Times    int           `env:"SIMPLE_WT_TIMES" env-default:"0"`
	Duration time.Duration `env:"SIMPLE_WT_DURATION" env-default:"1h"`
	Check    bool          `env:"SIMPLE_WT_CHECK" env-default:"false"`
}

type SimpleWithSkip struct {
	Skip string `env:"-"`
}

type ComplexEmbedded struct {
	EmbeddedString string `env:"CN_EMBEDDED_STRING"`
}

type ComplexInner struct {
	String string `env:"STRING"`
}

type ComplexNative struct {
	ComplexEmbedded

	String       string  `env:"CN_STRING"`
	Bool         bool    `env:"CN_BOOL"`
	BoolOptional *bool   `env:"CN_BOOL_OPTIONAL"`
	Int          int     `env:"CN_INT"`
	Int8         int8    `env:"CN_INT8"`
	Int16        int16   `env:"CN_INT16"`
	Int32        int32   `env:"CN_INT32"`
	Int64        int64   `env:"CN_INT64"`
	UInt         uint    `env:"CN_UINT"`
	UInt8        uint8   `env:"CN_UINT8"`
	UInt16       uint16  `env:"CN_UINT16"`
	UInt32       uint32  `env:"CN_UINT32"`
	UInt64       uint64  `env:"CN_UINT64"`
	Float32      float32 `env:"CN_FLOAT32"`
	Float64      float64 `env:"CN_FLOAT64"`
	IntArray     [3]int  `env:"CN_INT_ARRAY"`
	IntSlice     []int   `env:"CN_INT_SLICE" env-required:"false"`

	Inner ComplexInner `env:"CN_INNER_"`
}

type TestUnmarshaller struct {
	value string
}

var _ env.Unmarshaller = &TestUnmarshaller{}

func (tu *TestUnmarshaller) UnmarshalEnv(state env.UnmarshalState) error {
	tu.value, _ = state.Read()
	return nil
}

type TestTextUnmarshaller struct {
	value string
}

var _ encoding.TextUnmarshaler = &TestTextUnmarshaller{}

func (tu *TestTextUnmarshaller) UnmarshalText(b []byte) error {
	tu.value = string(b)
	return nil
}

type TestValidator string

var _ env.Validator = TestValidator("")

func (tu TestValidator) ValidateEnv(state env.UnmarshalState) error {
	if tu != "valid" {
		return errors.New("valid is only valid value")
	}
	return nil
}

type ComplexUnmarshal struct {
	A TestUnmarshaller     `env:"CU_A"`
	B TestTextUnmarshaller `env:"CU_B"`
	C TestValidator        `env:"CU_C"`
}

type TestMissing struct {
	Internal *TestMissingInternal `env:""`
}

type TestMissingInternal struct {
	Input string `env:"TMI_INPUT"`
}

type TestMultiple struct {
	Input string `env:"TM_IN,TM_INPUT"`
}

type TestAbsoluteInner struct {
	User string `env:"^TAI_USER"`
	Pass string `env:"PASS"`
}

type TestAbsolute struct {
	Inner TestAbsoluteInner `env:"TA_"`
}

type TestExplodeInner struct {
	Pass string `env:"PASS,PASSWORD"`
	User string `env:"USER,USERNAME" env-default:"sa"`
}

type TestExplode struct {
	Conn TestExplodeInner `env:"DB_,DATABASE_"`
}

func TestCases(t *testing.T) {
	cases := []struct {
		name          string
		set           map[string]string
		get           func() (any, error)
		check         func(t *testing.T, value any)
		expectedError string
	}{
		{
			name: "SimpleWithoutTags",
			set: map[string]string{
				"WITHOUT_TAGS": "success",
			},
			get: func() (any, error) {
				return env.Load[SimpleWithoutTags]()
			},
			check: func(t *testing.T, value any) {
				actual := value.(SimpleWithoutTags)
				assert.Equal(t, "success", actual.WITHOUT_TAGS)
			},
		},
		{
			name: "SimpleWithTags all success",
			set: map[string]string{
				"SIMPLE_WT_TEXT":     "abc",
				"SIMPLE_WT_TIMES":    "3",
				"SIMPLE_WT_DURATION": "2m",
				"SIMPLE_WT_CHECK":    "true",
			},
			get: func() (any, error) {
				return env.Load[SimpleWithTags]()
			},
			check: func(t *testing.T, value any) {
				actual := value.(SimpleWithTags)
				assert.Equal(t, "abc", actual.Text)
				assert.Equal(t, 3, actual.Times)
				assert.Equal(t, 2*time.Minute, actual.Duration)
				assert.Equal(t, true, actual.Check)
			},
		},
		{
			name: "SimpleWithTags required success",
			set: map[string]string{
				"SIMPLE_WT_TEXT": "abc",
			},
			get: func() (any, error) {
				return env.Load[SimpleWithTags]()
			},
			check: func(t *testing.T, value any) {
				actual := value.(SimpleWithTags)
				assert.Equal(t, "abc", actual.Text)
				assert.Equal(t, 0, actual.Times)
				assert.Equal(t, time.Hour, actual.Duration)
				assert.Equal(t, false, actual.Check)
			},
		},
		{
			name: "SimpleWithTags failure required",
			set:  map[string]string{},
			get: func() (any, error) {
				return env.Load[SimpleWithTags]()
			},
			expectedError: "SIMPLE_WT_TEXT: required",
		},
		{
			name: "SimpleWithTags failure format",
			set: map[string]string{
				"SIMPLE_WT_TEXT":  "",
				"SIMPLE_WT_TIMES": "a",
			},
			get: func() (any, error) {
				return env.Load[SimpleWithTags]()
			},
			expectedError: `SIMPLE_WT_TIMES: strconv.ParseInt: parsing "a": invalid syntax`,
		},
		{
			name: "SimpleWithSkip",
			set:  map[string]string{},
			get: func() (any, error) {
				skipped := SimpleWithSkip{Skip: "not touched"}
				return skipped, env.Parse(&skipped)
			},
			check: func(t *testing.T, value any) {
				actual := value.(SimpleWithSkip)
				assert.Equal(t, "not touched", actual.Skip)
			},
		},
		{
			name: "ComplexNative success full",
			set: map[string]string{
				"CN_EMBEDDED_STRING": "abc123",
				"CN_STRING":          "xyz",
				"CN_BOOL":            "true",
				"CN_BOOL_OPTIONAL":   "false",
				"CN_INT":             "-1",
				"CN_INT8":            "-2",
				"CN_INT16":           "-3",
				"CN_INT32":           "-4",
				"CN_INT64":           "-5",
				"CN_UINT":            "6",
				"CN_UINT8":           "7",
				"CN_UINT16":          "8",
				"CN_UINT32":          "9",
				"CN_UINT64":          "10",
				"CN_FLOAT32":         "-3.2",
				"CN_FLOAT64":         "6.4",
				"CN_INT_ARRAY":       "1,2,3",
				"CN_INT_SLICE":       "1,2,3,4,5",
				"CN_INNER_STRING":    "rst",
			},
			get: func() (any, error) {
				return env.Load[ComplexNative]()
			},
			check: func(t *testing.T, value any) {
				f := false
				actual := value.(ComplexNative)
				assert.Equal(t, "abc123", actual.EmbeddedString)
				assert.Equal(t, "xyz", actual.String)
				assert.Equal(t, true, actual.Bool)
				assert.Equal(t, &f, actual.BoolOptional)
				assert.Equal(t, -1, actual.Int)
				assert.Equal(t, int8(-2), actual.Int8)
				assert.Equal(t, int16(-3), actual.Int16)
				assert.Equal(t, int32(-4), actual.Int32)
				assert.Equal(t, int64(-5), actual.Int64)
				assert.Equal(t, uint(6), actual.UInt)
				assert.Equal(t, uint8(7), actual.UInt8)
				assert.Equal(t, uint16(8), actual.UInt16)
				assert.Equal(t, uint32(9), actual.UInt32)
				assert.Equal(t, uint64(10), actual.UInt64)
				assert.Equal(t, float32(-3.2), actual.Float32)
				assert.Equal(t, float64(6.4), actual.Float64)
				assert.Equal(t, [3]int{1, 2, 3}, actual.IntArray)
				assert.Equal(t, []int{1, 2, 3, 4, 5}, actual.IntSlice)
				assert.Equal(t, "rst", actual.Inner.String)
			},
		},
		{
			name: "ComplexNative success empty",
			set: map[string]string{
				"CN_EMBEDDED_STRING": "",
				"CN_STRING":          "",
				"CN_BOOL":            "true",
				"CN_INT":             "-1",
				"CN_INT8":            "-2",
				"CN_INT16":           "-3",
				"CN_INT32":           "-4",
				"CN_INT64":           "-5",
				"CN_UINT":            "6",
				"CN_UINT8":           "7",
				"CN_UINT16":          "8",
				"CN_UINT32":          "9",
				"CN_UINT64":          "10",
				"CN_FLOAT32":         "-3.2",
				"CN_FLOAT64":         "6.4",
				"CN_INT_ARRAY":       "1,2,3",
				"CN_INNER_STRING":    "rst",
			},
			get: func() (any, error) {
				return env.Load[ComplexNative]()
			},
			check: func(t *testing.T, value any) {
				actual := value.(ComplexNative)
				assert.Equal(t, "", actual.EmbeddedString)
				assert.Equal(t, "", actual.String)
				assert.Equal(t, true, actual.Bool)
				assert.Equal(t, (*bool)(nil), actual.BoolOptional)
				assert.Equal(t, -1, actual.Int)
				assert.Equal(t, int8(-2), actual.Int8)
				assert.Equal(t, int16(-3), actual.Int16)
				assert.Equal(t, int32(-4), actual.Int32)
				assert.Equal(t, int64(-5), actual.Int64)
				assert.Equal(t, uint(6), actual.UInt)
				assert.Equal(t, uint8(7), actual.UInt8)
				assert.Equal(t, uint16(8), actual.UInt16)
				assert.Equal(t, uint32(9), actual.UInt32)
				assert.Equal(t, uint64(10), actual.UInt64)
				assert.Equal(t, float32(-3.2), actual.Float32)
				assert.Equal(t, float64(6.4), actual.Float64)
				assert.Equal(t, [3]int{1, 2, 3}, actual.IntArray)
				assert.Equal(t, ([]int)(nil), actual.IntSlice)
				assert.Equal(t, "rst", actual.Inner.String)
			},
		},
		{
			name: "ComplexUnmarshal success",
			set: map[string]string{
				"CU_A": "hi",
				"CU_B": "hello world",
				"CU_C": "valid",
			},
			get: func() (any, error) {
				return env.Load[ComplexUnmarshal]()
			},
			check: func(t *testing.T, value any) {
				actual := value.(ComplexUnmarshal)
				assert.Equal(t, "hi", actual.A.value)
				assert.Equal(t, "hello world", actual.B.value)
				assert.Equal(t, TestValidator("valid"), actual.C)
			},
		},
		{
			name: "ComplexUnmarshal validation failure",
			set: map[string]string{
				"CU_A": "hi",
				"CU_B": "hello world",
				"CU_C": "invalid",
			},
			get: func() (any, error) {
				return env.Load[ComplexUnmarshal]()
			},
			expectedError: "CU_C: valid is only valid value",
		},
		{
			name: "TestMissing success",
			set: map[string]string{
				"TMI_INPUT": "hi",
			},
			get: func() (any, error) {
				return env.Load[TestMissing]()
			},
			check: func(t *testing.T, value any) {
				actual := value.(TestMissing)
				assert.Equal(t, "hi", actual.Internal.Input)
			},
		},
		{
			name: "TestMissing success no value",
			set:  map[string]string{},
			get: func() (any, error) {
				return env.Load[TestMissing]()
			},
			check: func(t *testing.T, value any) {
				actual := value.(TestMissing)
				assert.Nil(t, actual.Internal)
			},
		},
		{
			name: "TestMultiple first non-empty",
			set: map[string]string{
				"TM_IN":    "3",
				"TM_INPUT": "6",
			},
			get: func() (any, error) {
				return env.Load[TestMultiple]()
			},
			check: func(t *testing.T, value any) {
				actual := value.(TestMultiple)
				assert.Equal(t, "3", actual.Input)
			},
		},
		{
			name: "TestMultiple first empty",
			set: map[string]string{
				"TM_IN":    "",
				"TM_INPUT": "6",
			},
			get: func() (any, error) {
				return env.Load[TestMultiple]()
			},
			check: func(t *testing.T, value any) {
				actual := value.(TestMultiple)
				assert.Equal(t, "", actual.Input)
			},
		},
		{
			name: "TestMultiple second fallback",
			set: map[string]string{
				"TM_INPUT": "6",
			},
			get: func() (any, error) {
				return env.Load[TestMultiple]()
			},
			check: func(t *testing.T, value any) {
				actual := value.(TestMultiple)
				assert.Equal(t, "6", actual.Input)
			},
		},
		{
			name: "TestMultiple error",
			set:  map[string]string{},
			get: func() (any, error) {
				return env.Load[TestMultiple]()
			},
			expectedError: "TM_IN,TM_INPUT: required",
		},
		{
			name: "TestAbsolute",
			set: map[string]string{
				"TAI_USER": "u",
				"TA_PASS":  "p",
			},
			get: func() (any, error) {
				return env.Load[TestAbsolute]()
			},
			check: func(t *testing.T, value any) {
				actual := value.(TestAbsolute)
				assert.Equal(t, "u", actual.Inner.User)
				assert.Equal(t, "p", actual.Inner.Pass)
			},
		},
		{
			name: "TestExplode all",
			set: map[string]string{
				"DB_PASS":           "a",
				"DB_PASSWORD":       "b",
				"DATABASE_PASS":     "c",
				"DATABASE_PASSWORD": "d",
			},
			get: func() (any, error) {
				return env.Load[TestExplode]()
			},
			check: func(t *testing.T, value any) {
				actual := value.(TestExplode)
				assert.Equal(t, "a", actual.Conn.Pass)
			},
		},
		{
			name: "TestExplode second",
			set: map[string]string{
				"DB_PASSWORD":       "b",
				"DATABASE_PASS":     "c",
				"DATABASE_PASSWORD": "d",
			},
			get: func() (any, error) {
				return env.Load[TestExplode]()
			},
			check: func(t *testing.T, value any) {
				actual := value.(TestExplode)
				assert.Equal(t, "b", actual.Conn.Pass)
			},
		},
		{
			name: "TestExplode third",
			set: map[string]string{
				"DATABASE_PASS":     "c",
				"DATABASE_PASSWORD": "d",
			},
			get: func() (any, error) {
				return env.Load[TestExplode]()
			},
			check: func(t *testing.T, value any) {
				actual := value.(TestExplode)
				assert.Equal(t, "c", actual.Conn.Pass)
			},
		},
		{
			name: "TestExplode fourth",
			set: map[string]string{
				"DATABASE_PASSWORD": "d",
			},
			get: func() (any, error) {
				return env.Load[TestExplode]()
			},
			check: func(t *testing.T, value any) {
				actual := value.(TestExplode)
				assert.Equal(t, "d", actual.Conn.Pass)
			},
		},
		{
			name: "TestExplode error",
			set:  map[string]string{},
			get: func() (any, error) {
				return env.Load[TestExplode]()
			},
			expectedError: "DB_PASS,DB_PASSWORD,DATABASE_PASS,DATABASE_PASSWORD: required",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			for k, v := range testCase.set {
				os.Setenv(k, v)
			}
			defer func() {
				for k := range testCase.set {
					os.Unsetenv(k)
				}
			}()

			actual, err := testCase.get()

			if err != nil && testCase.expectedError == "" {
				t.Fatalf("unexpected error %v", err)
			} else if err == nil && testCase.expectedError != "" {
				t.Fatalf("expected error %s, got none", testCase.expectedError)
			} else if err != nil && testCase.expectedError != "" {
				if err.Error() != testCase.expectedError {
					t.Fatalf("expected error %s, got %s", testCase.expectedError, err)
				}
			} else {
				testCase.check(t, actual)
			}
		})
	}
}
