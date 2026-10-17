package paradedb

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"time"

	"github.com/superdurable/xtemporal/common/searchattribute"
	"github.com/superdurable/xtemporal/common/searchattribute/sadefs"
	commonpb "go.temporal.io/api/common/v1"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
)

const maxSearchAttributeStringLength = 32766

type encodedSearchAttributes struct {
	keyword  []byte
	text     []byte
	number   []byte
	datetime []byte
	boolean  []byte
	flowType *string
	division *string
}

func searchAttributeType(types searchattribute.NameTypeMap, name string) (enumspb.IndexedValueType, error) {
	t, err := types.GetType(name)
	if err == nil {
		return t, nil
	}
	if !sadefs.IsChasmSearchAttribute(name) {
		return 0, err
	}
	for _, item := range []struct {
		prefix string
		typeID enumspb.IndexedValueType
	}{
		{"TemporalBool", enumspb.INDEXED_VALUE_TYPE_BOOL},
		{"TemporalDatetime", enumspb.INDEXED_VALUE_TYPE_DATETIME},
		{"TemporalInt", enumspb.INDEXED_VALUE_TYPE_INT},
		{"TemporalDouble", enumspb.INDEXED_VALUE_TYPE_DOUBLE},
		{"TemporalText", enumspb.INDEXED_VALUE_TYPE_TEXT},
		{"TemporalKeywordList", enumspb.INDEXED_VALUE_TYPE_KEYWORD_LIST},
		{"TemporalLowCardinalityKeyword", enumspb.INDEXED_VALUE_TYPE_KEYWORD},
		{"TemporalKeyword", enumspb.INDEXED_VALUE_TYPE_KEYWORD},
	} {
		if len(name) >= len(item.prefix) && name[:len(item.prefix)] == item.prefix {
			return item.typeID, nil
		}
	}
	return 0, err
}

func encodeSearchAttributes(attrs *commonpb.SearchAttributes, types searchattribute.NameTypeMap) (encodedSearchAttributes, error) {
	buckets := map[string]map[string]any{
		colSAKeyword: {}, colSAText: {}, colSANumber: {}, colSADatetime: {}, colSABool: {},
	}
	result := encodedSearchAttributes{}
	for name, payload := range attrs.GetIndexedFields() {
		if err := encodeOneSearchAttribute(name, payload, types, buckets, &result); err != nil {
			return result, err
		}
	}
	var err error
	if result.keyword, err = json.Marshal(buckets[colSAKeyword]); err != nil {
		return result, err
	}
	if result.text, err = json.Marshal(buckets[colSAText]); err != nil {
		return result, err
	}
	if result.number, err = json.Marshal(buckets[colSANumber]); err != nil {
		return result, err
	}
	if result.datetime, err = json.Marshal(buckets[colSADatetime]); err != nil {
		return result, err
	}
	if result.boolean, err = json.Marshal(buckets[colSABool]); err != nil {
		return result, err
	}
	return result, nil
}

func encodeOneSearchAttribute(
	name string,
	payload *commonpb.Payload,
	types searchattribute.NameTypeMap,
	buckets map[string]map[string]any,
	result *encodedSearchAttributes,
) error {
	t, err := searchAttributeType(types, name)
	if err != nil {
		return serviceerror.NewInvalidArgumentf("unregistered search attribute %q", name)
	}
	value, err := sadefs.DecodeValue(payload, t, true)
	if err != nil {
		return serviceerror.NewInvalidArgumentf("invalid search attribute %q: %v", name, err)
	}
	if value == nil || isEmptyList(value) {
		return nil
	}
	if name == flowTypeSearchAttribute || name == sadefs.TemporalNamespaceDivision {
		str, ok := value.(string)
		if !ok {
			return serviceerror.NewInvalidArgumentf("%s must be a scalar keyword", name)
		}
		if err := validateSAString(str); err != nil {
			return err
		}
		if str != "" {
			if name == flowTypeSearchAttribute {
				result.flowType = &str
			} else {
				result.division = &str
			}
		}
		return nil
	}
	column, ok := searchAttributeColumn(t)
	if !ok {
		return serviceerror.NewInvalidArgumentf("unsupported search attribute type for %q", name)
	}
	value, err = normalizeSAValue(value, t)
	if err != nil {
		return serviceerror.NewInvalidArgumentf("invalid search attribute %q: %v", name, err)
	}
	buckets[column][name] = value
	return nil
}

func isEmptyList(value any) bool {
	v := reflect.ValueOf(value)
	return v.Kind() == reflect.Slice && v.Len() == 0
}

func validateSAString(value string) error {
	if len(value) > maxSearchAttributeStringLength {
		return serviceerror.NewInvalidArgumentf("search attribute string exceeds %d bytes", maxSearchAttributeStringLength)
	}
	return nil
}

func normalizeSAValue(value any, t enumspb.IndexedValueType) (any, error) {
	switch t {
	case enumspb.INDEXED_VALUE_TYPE_KEYWORD, enumspb.INDEXED_VALUE_TYPE_TEXT:
		return normalizeStringSA(value, false)
	case enumspb.INDEXED_VALUE_TYPE_KEYWORD_LIST:
		return normalizeStringSA(value, true)
	case enumspb.INDEXED_VALUE_TYPE_DATETIME:
		return normalizeDatetimeSA(value)
	case enumspb.INDEXED_VALUE_TYPE_INT:
		switch value.(type) {
		case int64, []int64:
			return value, nil
		}
	case enumspb.INDEXED_VALUE_TYPE_DOUBLE:
		return normalizeDoubleSA(value)
	case enumspb.INDEXED_VALUE_TYPE_BOOL:
		switch value.(type) {
		case bool, []bool:
			return value, nil
		}
	default:
		return nil, fmt.Errorf("unsupported search attribute type %s", t)
	}
	return nil, fmt.Errorf("unexpected value type %T for %s", value, t)
}

func normalizeStringSA(value any, requireList bool) (any, error) {
	switch v := value.(type) {
	case string:
		if requireList {
			return nil, errors.New("KeywordList must be an array")
		}
		return v, validateSAString(v)
	case []string:
		for _, item := range v {
			if err := validateSAString(item); err != nil {
				return nil, err
			}
		}
		return v, nil
	default:
		return nil, fmt.Errorf("expected string or []string, got %T", value)
	}
}

func normalizeDatetimeSA(value any) (any, error) {
	switch v := value.(type) {
	case time.Time:
		return datetimeMicros(v)
	case []time.Time:
		out := make([]int64, len(v))
		for i, item := range v {
			var err error
			out[i], err = datetimeMicros(item)
			if err != nil {
				return nil, err
			}
		}
		return out, nil
	default:
		return nil, fmt.Errorf("expected datetime or []datetime, got %T", value)
	}
}

func normalizeDoubleSA(value any) (any, error) {
	switch v := value.(type) {
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, errors.New("invalid double")
		}
		return v, nil
	case []float64:
		for _, item := range v {
			if math.IsNaN(item) || math.IsInf(item, 0) {
				return nil, errors.New("invalid double")
			}
		}
		return v, nil
	default:
		return nil, fmt.Errorf("expected double or []double, got %T", value)
	}
}

func datetimeMicros(value time.Time) (int64, error) {
	if value.Before(time.Unix(0, 0)) || value.After(time.Unix(0, math.MaxInt64)) {
		return 0, errors.New("datetime outside supported range")
	}
	return value.UTC().UnixMicro(), nil
}

func decodeSearchAttributes(encoded encodedSearchAttributes, types searchattribute.NameTypeMap) (*commonpb.SearchAttributes, error) {
	fields := make(map[string]*commonpb.Payload)
	for _, bucket := range []struct {
		column string
		data   []byte
	}{
		{colSAKeyword, encoded.keyword}, {colSAText, encoded.text}, {colSANumber, encoded.number},
		{colSADatetime, encoded.datetime}, {colSABool, encoded.boolean},
	} {
		if err := decodeSABucket(fields, bucket.column, bucket.data, types); err != nil {
			return nil, err
		}
	}
	if err := decodePromotedSA(fields, flowTypeSearchAttribute, encoded.flowType, types); err != nil {
		return nil, err
	}
	if err := decodePromotedSA(fields, sadefs.TemporalNamespaceDivision, encoded.division, types); err != nil {
		return nil, err
	}
	if len(fields) == 0 {
		return nil, nil
	}
	return &commonpb.SearchAttributes{IndexedFields: fields}, nil
}

func decodeSABucket(
	fields map[string]*commonpb.Payload,
	column string,
	data []byte,
	types searchattribute.NameTypeMap,
) error {
	if len(data) == 0 {
		return nil
	}
	var values map[string]json.RawMessage
	if err := json.Unmarshal(data, &values); err != nil {
		return fmt.Errorf("decode %s: %w", column, err)
	}
	for name, raw := range values {
		t, err := searchAttributeType(types, name)
		if err != nil {
			continue
		}
		bucket, ok := searchAttributeColumn(t)
		if !ok || bucket != column {
			return fmt.Errorf("search attribute %q has wrong JSONB bucket", name)
		}
		value, err := decodeSAValue(raw, t)
		if err != nil {
			return fmt.Errorf("decode search attribute %q: %w", name, err)
		}
		if value == nil || isEmptyList(value) {
			continue
		}
		fields[name], err = sadefs.EncodeValue(value, t)
		if err != nil {
			return fmt.Errorf("encode search attribute %q: %w", name, err)
		}
	}
	return nil
}

func decodePromotedSA(
	fields map[string]*commonpb.Payload,
	name string,
	value *string,
	types searchattribute.NameTypeMap,
) error {
	if value == nil || *value == "" {
		return nil
	}
	t, err := searchAttributeType(types, name)
	if err != nil && name != sadefs.TemporalNamespaceDivision {
		return nil
	}
	if err != nil {
		t = enumspb.INDEXED_VALUE_TYPE_KEYWORD
	}
	fields[name], err = sadefs.EncodeValue(*value, t)
	return err
}

func decodeSAValue(raw json.RawMessage, t enumspb.IndexedValueType) (any, error) {
	if string(raw) == "null" {
		return nil, nil
	}
	list := len(raw) > 0 && raw[0] == '['
	if t == enumspb.INDEXED_VALUE_TYPE_KEYWORD_LIST && !list {
		return nil, errors.New("KeywordList must be an array")
	}
	switch t {
	case enumspb.INDEXED_VALUE_TYPE_KEYWORD, enumspb.INDEXED_VALUE_TYPE_KEYWORD_LIST, enumspb.INDEXED_VALUE_TYPE_TEXT:
		if list {
			var v []string
			err := json.Unmarshal(raw, &v)
			return v, err
		}
		var v string
		err := json.Unmarshal(raw, &v)
		return v, err
	case enumspb.INDEXED_VALUE_TYPE_INT:
		if list {
			var v []int64
			err := json.Unmarshal(raw, &v)
			return v, err
		}
		var v int64
		err := json.Unmarshal(raw, &v)
		return v, err
	case enumspb.INDEXED_VALUE_TYPE_DOUBLE:
		if list {
			var v []float64
			err := json.Unmarshal(raw, &v)
			return v, err
		}
		var v float64
		err := json.Unmarshal(raw, &v)
		return v, err
	case enumspb.INDEXED_VALUE_TYPE_BOOL:
		if list {
			var v []bool
			err := json.Unmarshal(raw, &v)
			return v, err
		}
		var v bool
		err := json.Unmarshal(raw, &v)
		return v, err
	case enumspb.INDEXED_VALUE_TYPE_DATETIME:
		if list {
			var micros []int64
			if err := json.Unmarshal(raw, &micros); err != nil {
				return nil, err
			}
			v := make([]time.Time, len(micros))
			for i, n := range micros {
				v[i] = time.UnixMicro(n).UTC()
			}
			return v, nil
		}
		var micros int64
		if err := json.Unmarshal(raw, &micros); err != nil {
			return nil, err
		}
		return time.UnixMicro(micros).UTC(), nil
	default:
		return nil, fmt.Errorf("unsupported search attribute type %s", t)
	}
}
