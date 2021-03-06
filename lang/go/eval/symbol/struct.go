//
// Copyright © 2018 Aljabr, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package symbol

import (
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/golang/protobuf/proto"

	"github.com/kocircuit/kocircuit/lang/circuit/eval"
	"github.com/kocircuit/kocircuit/lang/circuit/model"
	pb "github.com/kocircuit/kocircuit/lang/go/eval/symbol/proto"
	"github.com/kocircuit/kocircuit/lang/go/gate"
	"github.com/kocircuit/kocircuit/lang/go/kit/tree"
)

func MakeStructSymbol(fields FieldSymbols) *StructSymbol {
	return &StructSymbol{
		Type_: &StructType{Field: FieldSymbolTypes(fields)},
		Field: fields,
	}
}

func FilterEmptyStructFields(ss *StructSymbol) *StructSymbol {
	return MakeStructSymbol(FilterEmptyFieldSymbols(ss.Field))
}

func FilterEmptyFieldSymbols(fields FieldSymbols) (filtered FieldSymbols) {
	filtered = make(FieldSymbols, 0, len(fields))
	for _, field := range fields {
		if !IsEmptySymbol(field.Value) {
			filtered = append(filtered, field)
		}
	}
	return
}

func FieldSymbolTypes(fields FieldSymbols) []*FieldType {
	types := make([]*FieldType, len(fields))
	for i, field := range fields {
		types[i] = &FieldType{
			Name:  field.Name,
			Type_: field.Value.Type(),
		}
	}
	return types
}

type StructSymbol struct {
	Type_ *StructType  `ko:"name=type"`
	Field FieldSymbols `ko:"name=field"`
}

var _ Symbol = &StructSymbol{}

type FieldSymbol struct {
	Name    string `ko:"name=name"`
	Monadic bool   `ko:"name=monadic"`
	Value   Symbol `ko:"name=value"`
}

func disassembleFieldSymbolsToGo(span *model.Span, fields FieldSymbols, st *StructType) (interface{}, error) {
	filtered := FilterEmptyFieldSymbols(fields)
	goType, koToGoNameMap := st.GoTypeAndNameMap()
	m := reflect.New(goType)
	for _, field := range filtered {
		value, err := field.Value.DisassembleToGo(span)
		if err != nil {
			return nil, err
		}
		if !isNil(value) {
			goName := koToGoNameMap[field.Name]
			m.Elem().FieldByName(goName).Set(value)
		}
	}
	return m.Interface(), nil
}

func disassembleFieldSymbolsToPB(span *model.Span, fields FieldSymbols) ([]*pb.SymbolField, error) {
	filtered := FilterEmptyFieldSymbols(fields)
	dis := make([]*pb.SymbolField, 0, len(filtered))
	for _, field := range filtered {
		value, err := field.Value.DisassembleToPB(span)
		if err != nil {
			return nil, err
		}
		if value != nil {
			dis = append(dis,
				&pb.SymbolField{
					Name:    proto.String(field.Name),
					Monadic: proto.Bool(field.Monadic),
					Value:   value,
				},
			)
		}
	}
	return dis, nil
}

// DisassembleToGo converts a Ko value into a Go value
func (ss *StructSymbol) DisassembleToGo(span *model.Span) (reflect.Value, error) {
	fields, err := disassembleFieldSymbolsToGo(span, ss.Field, ss.Type_)
	if err != nil {
		return reflect.Value{}, err
	}
	return reflect.ValueOf(fields), nil
}

// DisassembleToPB converts a Ko value into a protobuf
func (ss *StructSymbol) DisassembleToPB(span *model.Span) (*pb.Symbol, error) {
	fields, err := disassembleFieldSymbolsToPB(span, ss.Field)
	if err != nil {
		return nil, err
	}
	dis := &pb.SymbolStruct{Field: fields}
	return &pb.Symbol{
		Symbol: &pb.Symbol_Struct{Struct: dis},
	}, nil
}

func (ss *StructSymbol) IsEmpty() bool {
	return len(ss.Field) == 0
}

func (ss *StructSymbol) String() string {
	return tree.Sprint(ss)
}

func (ss *StructSymbol) Equal(span *model.Span, sym Symbol) bool {
	if other, ok := sym.(*StructSymbol); ok {
		return FieldSymbolsEqual(span, ss.Field, other.Field)
	} else {
		return false
	}
}

func FieldSymbolsEqual(span *model.Span, x, y FieldSymbols) bool {
	x, y = FilterEmptyFieldSymbols(x), FilterEmptyFieldSymbols(y)
	if len(x) != len(y) {
		return false
	}
	u, v := x.Copy(), y.Copy()
	u.Sort()
	v.Sort()
	for i := range u {
		if u[i].Name != v[i].Name || !u[i].Value.Equal(span, v[i].Value) {
			return false
		}
	}
	return true
}

func (ss *StructSymbol) Hash(span *model.Span) model.ID {
	return FieldSymbolsHash(span, ss.Field)
}

func FieldSymbolsHash(span *model.Span, fields FieldSymbols) model.ID {
	fields = FilterEmptyFieldSymbols(fields)
	h := make([]model.ID, 2*len(fields))
	for i, field := range fields {
		h[2*i] = model.StringID(field.Name)
		h[2*i+1] = field.Value.Hash(span)
	}
	return model.Blend(h...)
}

func (ss *StructSymbol) LiftToSeries(span *model.Span) *SeriesSymbol {
	return singletonSeries(ss)
}

func (ss *StructSymbol) Augment(span *model.Span, _ eval.Fields) (eval.Shape, eval.Effect, error) {
	return nil, nil, span.Errorf(nil, "structure %v cannot be augmented", ss)
}

func (ss *StructSymbol) Invoke(span *model.Span) (eval.Shape, eval.Effect, error) {
	return nil, nil, span.Errorf(nil, "structure %v cannot be invoked", ss)
}

func (ss *StructSymbol) Type() Type {
	return ss.Type_
}

func (ss *StructSymbol) Splay() tree.Tree {
	nameTrees := make([]tree.NameTree, len(ss.Field))
	for i, field := range ss.Field {
		nameTrees[i] = tree.NameTree{
			Name:    gate.KoGoName{Ko: field.Name},
			Monadic: field.Monadic,
			Tree:    field.Value.Splay(),
		}
	}
	return tree.Parallel{
		Label:   tree.Label{Path: "", Name: ""},
		Bracket: "()",
		Elem:    nameTrees,
	}
}

func (ss *StructSymbol) FindMonadic() *FieldSymbol {
	for _, fs := range ss.Field {
		if fs.Monadic || fs.Name == "" {
			return fs
		}
	}
	return nil
}

func (ss *StructSymbol) FindName(name string) *FieldSymbol {
	for _, fs := range ss.Field {
		if fs.Name == name {
			return fs
		}
	}
	return nil
}

type FieldSymbols []*FieldSymbol

func (fs FieldSymbols) Copy() FieldSymbols {
	c := make(FieldSymbols, len(fs))
	copy(c, fs)
	return c
}

func (fs FieldSymbols) Sort() {
	sort.Sort(fs)
}

func (fs FieldSymbols) Len() int {
	return len(fs)
}

func (fs FieldSymbols) Less(i, j int) bool {
	return fs[i].Name < fs[j].Name
}

func (fs FieldSymbols) Swap(i, j int) {
	fs[i], fs[j] = fs[j], fs[i]
}

type StructType struct {
	Field []*FieldType `ko:"name=field"`
}

type FieldType struct {
	Name  string `ko:"name=name"`
	Type_ Type   `ko:"name=type"`
}

var _ Type = &StructType{}

func (*StructType) IsType() {}

func (st *StructType) String() string {
	return tree.Sprint(st)
}

func (st *StructType) Splay() tree.Tree {
	nameTrees := make([]tree.NameTree, len(st.Field))
	for i, field := range st.Field {
		nameTrees[i] = tree.NameTree{
			Name: gate.KoGoName{Ko: field.Name},
			Tree: field.Type_.Splay(),
		}
	}
	return tree.Parallel{
		Label:   tree.Label{Path: "", Name: ""},
		Bracket: "()",
		Elem:    nameTrees,
	}
}

// GoType returns the Go equivalent of the type.
func (st *StructType) GoType() reflect.Type {
	goType, _ := st.GoTypeAndNameMap()
	return goType
}

// GoTypeAndNameMap returns the Go equivalent of the type
// and a map from Ko (field) name to Go (field) name
func (st *StructType) GoTypeAndNameMap() (reflect.Type, map[string]string) {
	fields := make([]reflect.StructField, 0, len(st.Field))
	koToGoNameMap := make(map[string]string)
	for i, f := range st.Field {
		goName := strings.ToUpper(f.Name[:1]) + f.Name[1:]
		// Search for clashing names
		for _, v := range koToGoNameMap {
			if v == goName {
				// Found a clash
				goName = goName + strconv.Itoa(i)
				break
			}
		}
		fields = append(fields, reflect.StructField{
			Name: goName,
			Type: f.Type_.GoType(),
			Tag:  reflect.StructTag(fmt.Sprintf(`ko:"name=%s" json:"%s"`, f.Name, f.Name)),
		})
		koToGoNameMap[f.Name] = goName
	}
	return reflect.StructOf(fields), koToGoNameMap
}
