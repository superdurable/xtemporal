package paradedb

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/superdurable/xtemporal/common/log"
	"github.com/superdurable/xtemporal/common/searchattribute"
	"github.com/superdurable/xtemporal/common/searchattribute/sadefs"
	commonpb "go.temporal.io/api/common/v1"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
)

func codecTypes() searchattribute.NameTypeMap {
	return searchattribute.NewNameTypeMap(map[string]enumspb.IndexedValueType{
		"FlowType": enumspb.INDEXED_VALUE_TYPE_KEYWORD,
		"Keyword":  enumspb.INDEXED_VALUE_TYPE_KEYWORD,
		"Keywords": enumspb.INDEXED_VALUE_TYPE_KEYWORD_LIST,
		"Text":     enumspb.INDEXED_VALUE_TYPE_TEXT,
		"Int":      enumspb.INDEXED_VALUE_TYPE_INT,
		"Double":   enumspb.INDEXED_VALUE_TYPE_DOUBLE,
		"Datetime": enumspb.INDEXED_VALUE_TYPE_DATETIME,
		"Bool":     enumspb.INDEXED_VALUE_TYPE_BOOL,
	})
}

func TestSearchAttributeCodecRoundTrip(t *testing.T) {
	date := time.Date(2025, 3, 4, 5, 6, 7, 123456789, time.FixedZone("offset", 3600))
	values := map[string]any{
		"Keyword": "alpha", "Keywords": []string{"a", "b"},
		"Text": []string{"one", "two"}, "Int": []int64{math.MaxInt64, -5},
		"Double": []float64{1.25, -2.5}, "Datetime": []time.Time{date},
		"Bool": []bool{true, false}, "FlowType": "invoice",
		sadefs.TemporalNamespaceDivision: "TemporalScheduler",
		"TemporalInt01":                  int64(42),
	}
	attrs := &commonpb.SearchAttributes{IndexedFields: make(map[string]*commonpb.Payload)}
	for name, value := range values {
		typeID, err := searchAttributeType(codecTypes(), name)
		require.NoError(t, err)
		attrs.IndexedFields[name], err = sadefs.EncodeValue(value, typeID)
		require.NoError(t, err)
	}
	encoded, err := encodeSearchAttributes(attrs, codecTypes())
	require.NoError(t, err)
	require.Equal(t, "invoice", *encoded.flowType)
	require.Equal(t, "TemporalScheduler", *encoded.division)
	require.NotContains(t, string(encoded.keyword), "FlowType")
	require.Contains(t, string(encoded.number), "9223372036854775807")
	decoded, err := decodeSearchAttributes(encoded, codecTypes())
	require.NoError(t, err)
	for name, want := range values {
		typeID, err := searchAttributeType(codecTypes(), name)
		require.NoError(t, err)
		got, err := sadefs.DecodeValue(decoded.IndexedFields[name], typeID, true)
		require.NoError(t, err)
		if name == "Datetime" {
			require.Equal(t, []time.Time{date.UTC().Truncate(time.Microsecond)}, got)
		} else {
			require.Equal(t, want, got)
		}
	}
}

func TestSearchAttributeCodecOmissionAndValidation(t *testing.T) {
	types := codecTypes()
	attrs := &commonpb.SearchAttributes{IndexedFields: map[string]*commonpb.Payload{}}
	var err error
	attrs.IndexedFields["Keyword"], err = sadefs.EncodeValue(nil, enumspb.INDEXED_VALUE_TYPE_KEYWORD)
	require.NoError(t, err)
	attrs.IndexedFields["Keywords"], err = sadefs.EncodeValue([]string{}, enumspb.INDEXED_VALUE_TYPE_KEYWORD_LIST)
	require.NoError(t, err)
	encoded, err := encodeSearchAttributes(attrs, types)
	require.NoError(t, err)
	require.JSONEq(t, `{}`, string(encoded.keyword))

	// A write skips what it cannot store instead of failing the visibility task.
	attrs.IndexedFields["Unknown"], err = sadefs.EncodeValue("value", enumspb.INDEXED_VALUE_TYPE_KEYWORD)
	require.NoError(t, err)
	attrs.IndexedFields["FlowType"], err = sadefs.EncodeValue([]string{"a"}, enumspb.INDEXED_VALUE_TYPE_KEYWORD)
	require.NoError(t, err)
	encoded, err = encodeSearchAttributes(attrs, types)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"Unknown", "FlowType"}, encoded.skipped)
	require.Nil(t, encoded.flowType)
	require.JSONEq(t, `{}`, string(encoded.keyword))
	delete(attrs.IndexedFields, "Unknown")
	delete(attrs.IndexedFields, "FlowType")
	store := &VisibilityStore{indexName: "test", saProvider: integrationProvider{}, logger: log.NewNoopLogger()}
	_, err = store.ValidateCustomSearchAttributes(map[string]any{"FlowType": []string{"a"}})
	require.ErrorAs(t, err, new(*serviceerror.InvalidArgument))
	_, err = store.ValidateCustomSearchAttributes(map[string]any{"Unknown": "value"})
	require.ErrorAs(t, err, new(*serviceerror.InvalidArgument))
	_, err = store.ValidateCustomSearchAttributes(map[string]any{"Datetime": time.Unix(-1, 0)})
	require.ErrorAs(t, err, new(*serviceerror.InvalidArgument))
	_, err = store.ValidateCustomSearchAttributes(map[string]any{"Keyword": string(make([]byte, maxSearchAttributeStringLength+1))})
	require.ErrorAs(t, err, new(*serviceerror.InvalidArgument))

	decoded, err := decodeSearchAttributes(encodedSearchAttributes{keyword: []byte(`{"Unknown":"value","Keyword":"ok"}`)}, types)
	require.NoError(t, err)
	require.Len(t, decoded.IndexedFields, 1)
	require.Contains(t, decoded.IndexedFields, "Keyword")
}
