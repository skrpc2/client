package rycli

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/apache/fory/go/fory"
)

const baseTypeName = "ryrpc.Base"

// PBase is the RPC response envelope.
type PBase struct {
	Err  string
	Data []byte
}

type typeEntry struct {
	proto reflect.Type
	name  string
}

type pooledCodec struct {
	pool            sync.Pool
	registeredTypes sync.Map // registration key -> typeEntry
	typeOnce        sync.Map // reflect type key -> *sync.Once
}

func newPooledCodec() *pooledCodec {
	c := &pooledCodec{}
	c.pool = sync.Pool{New: func() any { return c.newFory() }}
	c.registeredTypes.Store(baseTypeName, typeEntry{
		proto: reflect.TypeOf(PBase{}),
		name:  baseTypeName,
	})
	return c
}

func (c *pooledCodec) newFory() *fory.Fory {
	f := fory.New(fory.WithXlang(false), fory.WithCompatible(true))
	c.applyAll(f)
	return f
}

func (c *pooledCodec) applyAll(f *fory.Fory) {
	c.registeredTypes.Range(func(_, value any) bool {
		entry := value.(typeEntry)
		inst := reflect.New(entry.proto).Interface()
		_ = f.RegisterStructByName(inst, entry.name)
		return true
	})
}

func (c *pooledCodec) acquire() *fory.Fory {
	f := c.pool.Get().(*fory.Fory)
	c.applyAll(f)
	return f
}

func (c *pooledCodec) release(f *fory.Fory) {
	f.Reset()
	c.pool.Put(f)
}

func (c *pooledCodec) Serialize(v any) ([]byte, error) {
	f := c.acquire()
	data, err := f.Serialize(v)
	if err != nil {
		c.release(f)
		return nil, err
	}
	out := make([]byte, len(data))
	copy(out, data)
	c.release(f)
	return out, nil
}

func (c *pooledCodec) Deserialize(data []byte, v any) error {
	f := c.acquire()
	defer c.release(f)
	return f.Deserialize(data, v)
}

var codec = newPooledCodec()

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
	onceVal, _ := codec.typeOnce.LoadOrStore(key, &sync.Once{})
	once := onceVal.(*sync.Once)
	var regErr error
	once.Do(func() {
		codec.registeredTypes.Store(key, typeEntry{proto: t, name: key})
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
