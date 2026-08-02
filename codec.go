package rycli

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/apache/fory/go/fory"
	"github.com/apache/fory/go/fory/threadsafe"
)

const baseTypeName = "ryrpc.Base"

// PBase is the RPC response envelope.
type PBase struct {
	Err  string
	Data []byte
}

var (
	codec   = threadsafe.New(fory.WithXlang(false), fory.WithCompatible(true))
	typeReg sync.Map // type key -> *sync.Once
)

func init() {
	if err := codec.RegisterStructByName(&PBase{}, baseTypeName); err != nil {
		panic(fmt.Sprintf("rycli: register PBase: %v", err))
	}
}

func typeKey(t reflect.Type) string {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.PkgPath() + "." + t.Name()
}

func ensureStructRegistered(t reflect.Type) error {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}
	key := typeKey(t)
	onceVal, _ := typeReg.LoadOrStore(key, &sync.Once{})
	once := onceVal.(*sync.Once)
	var regErr error
	once.Do(func() {
		inst := reflect.New(t).Interface()
		regErr = codec.RegisterStructByName(inst, key)
	})
	return regErr
}

func marshalPayload(v interface{}) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	switch val := v.(type) {
	case string:
		return []byte(val), nil
	case []byte:
		return val, nil
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Ptr:
		if rv.IsNil() {
			return codec.Serialize(nil)
		}
		if err := ensureStructRegistered(rv.Type()); err != nil {
			return nil, err
		}
		return codec.Serialize(v)
	case reflect.Struct:
		if err := ensureStructRegistered(rv.Type()); err != nil {
			return nil, err
		}
		ptr := reflect.New(rv.Type())
		ptr.Elem().Set(rv)
		return codec.Serialize(ptr.Interface())
	default:
		return codec.Serialize(v)
	}
}

func unmarshalPayload(data []byte, v interface{}) error {
	if v == nil {
		return fmt.Errorf("rycli: unmarshal target is nil")
	}
	if sp, ok := v.(*string); ok {
		*sp = string(data)
		return nil
	}
	if bp, ok := v.(*[]byte); ok {
		*bp = data
		return nil
	}

	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return fmt.Errorf("rycli: unmarshal target must be a non-nil pointer")
	}
	if err := ensureStructRegistered(rv.Type()); err != nil {
		return err
	}
	return codec.Deserialize(data, v)
}

func unmarshalEnvelope(data []byte) (*PBase, error) {
	args := &PBase{}
	if err := codec.Deserialize(data, args); err != nil {
		return nil, err
	}
	return args, nil
}
