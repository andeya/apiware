package apiware

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParseTags(t *testing.T) {
	m := parseTags(`type(path),required,desc(banana)`)
	if x, ok := m["required"]; !ok {
		t.Fatal("wrong value", ok, x)
	}
	if x, ok := m["desc"]; !ok || x != "banana" {
		t.Fatal("wrong value", x)
	}
}

func TestFieldIsZero(t *testing.T) {
	field := &StructField{}
	field.Value = reflect.ValueOf(0)
	if !field.IsZero() {
		t.Fatal("should be zero")
	}
	field.Value = reflect.ValueOf("")
	if !field.IsZero() {
		t.Fatal("should be zero")
	}
	field.Value = reflect.ValueOf(false)
	if !field.IsZero() {
		t.Fatal("should be zero")
	}
	field.Value = reflect.ValueOf(true)
	if field.IsZero() {
		t.Fatal("should not be zero")
	}
	field.Value = reflect.ValueOf(-1)
	if field.IsZero() {
		t.Fatal("should not be zero")
	}
	field.Value = reflect.ValueOf(1)
	if field.IsZero() {
		t.Fatal("should not be zero")
	}
	field.Value = reflect.ValueOf("asdf")
	if field.IsZero() {
		t.Fatal("should not be zero")
	}
}

func TestFieldValidate(t *testing.T) {
	type Schema struct {
		A string  `param:"type(path),len(3:6),name(p)"`
		B float32 `param:"type(query),range(10:20)"`
		C string  `param:"type(query),len(:4),nonzero"`
		D string  `param:"type(query)" regexp:"^[a-zA-Z0-9_.+-]+@[a-zA-Z0-9-]+\\.[a-zA-Z0-9-.]+$"`
	}
	m, _ := ToStruct(&Schema{B: 9.999999}, toSnake)
	a := m.Fields[0]
	if x := len(a.Tags); x != 4 {
		t.Fatal("wrong len", x, a.Tags)
	}
	if x, ok := a.Tags["len"]; !ok || x != "3:6" {
		t.Fatal("wrong value", x, ok)
	}
	if err := a.Validate(); err == nil || err.Error() != "p too short" {
		t.Fatal("should not validate")
	}
	a.Value = reflect.ValueOf("abc")
	if err := a.Validate(); err != nil {
		t.Fatal("should validate", err)
	}
	a.Value = reflect.ValueOf("abcdefg")
	if err := a.Validate(); err == nil || err.Error() != "p too long" {
		t.Fatal("should not validate")
	}

	b := m.Fields[1]
	if x := len(b.Tags); x != 2 {
		t.Fatal("wrong len", x)
	}
	if err := b.Validate(); err == nil || err.Error() != "b too small" {
		t.Fatal("should not validate")
	}
	b.Value = reflect.ValueOf(10)
	if err := b.Validate(); err != nil {
		t.Fatal("should validate", err)
	}
	b.Value = reflect.ValueOf(21)
	if err := b.Validate(); err == nil || err.Error() != "b too big" {
		t.Fatal("should not validate")
	}

	c := m.Fields[2]
	if x := len(c.Tags); x != 3 {
		t.Fatal("wrong len", x)
	}
	if err := c.Validate(); err == nil || err.Error() != "c not set" {
		t.Fatal("should not validate")
	}
	c.Value = reflect.ValueOf("a")
	if err := c.Validate(); err != nil {
		t.Fatal("should validate", err)
	}
	c.Value = reflect.ValueOf("abcde")
	if err := c.Validate(); err == nil || err.Error() != "c too long" {
		t.Fatal("should not validate")
	}

	d := m.Fields[3]
	if x := len(d.Tags); x != 2 {
		t.Fatal("wrong len", x)
	}
	d.Value = reflect.ValueOf("gggg@gmail.com")
	if err := d.Validate(); err != nil {
		t.Fatal("should validate", err)
	}
	d.Value = reflect.ValueOf("www.google.com")
	if err := d.Validate(); err == nil || err.Error() != "d not match" {
		t.Fatal("should not validate", err)
	}
}

func TestFieldOmit(t *testing.T) {
	type schema struct {
		A string `param:"-"`
		B string
	}
	m, _ := ToStruct(&schema{}, toSnake)
	if x := len(m.Fields); x != 0 {
		t.Fatal("wrong len", x)
	}
}

func TestInterfaceToStructWithEmbedded(t *testing.T) {
	type embed struct {
		Name  string `param:"type(query)"`
		Value string `param:"type(query)"`
	}
	type table struct {
		ColPrimary int64 `param:"type(query)"`
		embed
	}
	table1 := &table{
		6, embed{"Mrs. A", "infinite"},
	}
	m, err := ToStruct(table1, toSnake)
	if err != nil {
		t.Fatal("error not nil", err)
	}
	f := m.Fields[1]
	if x, ok := f.String(); !ok || x != "Mrs. A" {
		t.Fatal("wrong value from embedded struct")
	}
}

type indexedTable struct {
	ColIsRequired string `param:"type(query),required"`
	ColVarChar    string `param:"type(query),desc(banana)"`
	ColTime       time.Time
}

func TestInterfaceToStruct(t *testing.T) {
	now := time.Now()
	table1 := &indexedTable{
		ColVarChar: "orange",
		ColTime:    now,
	}
	m, err := ToStruct(table1, toSnake)
	if err != nil {
		t.Fatal("error not nil", err)
	}
	if x := len(m.Fields); x != 2 {
		t.Fatal("wrong value", x)
	}
	f := m.Fields[0]
	if !f.IsRequired() {
		t.Fatal("wrong value")
	}
	f = m.Fields[1]
	if x, ok := f.String(); !ok || x != "orange" {
		t.Fatal("wrong value", x)
	}
	if f.IsZero() {
		t.Fatal("wrong value")
	}
	if f.Description() != "banana" {
		t.Fatal("should value", f.Description())
	}
	if f.IsRequired() {
		t.Fatal("wrong value")
	}
}

func makeWhitespaceVisible(s string) string {
	s = strings.Replace(s, "\t", "\\t", -1)
	s = strings.Replace(s, "\r\n", "\\r\\n", -1)
	s = strings.Replace(s, "\r", "\\r", -1)
	s = strings.Replace(s, "\n", "\\n", -1)
	return s
}
