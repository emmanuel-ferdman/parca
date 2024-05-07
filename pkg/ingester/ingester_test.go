// Copyright 2022-2024 The Parca Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ingester

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"os"
	"testing"

	"github.com/apache/arrow/go/v16/arrow"
	"github.com/apache/arrow/go/v16/arrow/memory"
	"github.com/go-kit/log"
	"github.com/polarsignals/frostdb/dynparquet"
	"github.com/stretchr/testify/require"

	profilestorepb "github.com/parca-dev/parca/gen/proto/go/parca/profilestore/v1alpha1"
	"github.com/parca-dev/parca/pkg/normalizer"
	"github.com/parca-dev/parca/pkg/profile"
)

func MustReadAllGzip(t require.TestingT, filename string) []byte {
	f, err := os.Open(filename)
	require.NoError(t, err)
	defer f.Close()

	r, err := gzip.NewReader(f)
	require.NoError(t, err)
	content, err := io.ReadAll(r)
	require.NoError(t, err)
	return content
}

type fakeTable struct {
	schema *dynparquet.Schema

	inserts []arrow.Record
}

func (t *fakeTable) Schema() *dynparquet.Schema {
	return t.schema
}

func (t *fakeTable) InsertRecord(ctx context.Context, record arrow.Record) (uint64, error) {
	record.Retain()
	t.inserts = append(t.inserts, record)
	return 0, nil
}

func TestPprofToArrow(t *testing.T) {
	logger := log.NewNopLogger()
	ctx := context.Background()

	schema, err := profile.Schema()
	require.NoError(t, err)

	fileContent, err := os.ReadFile("../query/testdata/alloc_objects.pb.gz")
	require.NoError(t, err)

	table := &fakeTable{
		schema: schema,
	}
	req := &profilestorepb.WriteRawRequest{
		Series: []*profilestorepb.RawProfileSeries{{
			Labels: &profilestorepb.LabelSet{
				Labels: []*profilestorepb.Label{
					{
						Name:  "__name__",
						Value: "memory",
					},
					{
						Name:  "job",
						Value: "default",
					},
				},
			},
			Samples: []*profilestorepb.RawSample{{
				RawProfile: fileContent,
			}},
		}},
	}
	normalizedReq, err := normalizer.NormalizeWriteRawRequest(ctx, normalizer.New(), req)
	require.NoError(t, err)
	ingester := NewIngester(logger, memory.DefaultAllocator, table, schema)
	err = ingester.Ingest(ctx, normalizedReq)
	require.NoError(t, err)
	require.Equal(t, 1, len(table.inserts))
	got, err := table.inserts[0].MarshalJSON()
	require.NoError(t, err)

	want, err := os.ReadFile("testdata/ingest_arrow.json")
	require.NoError(t, err)
	require.JSONEq(t, string(want), string(got))
}

func TestUncompressedPprofToArrow(t *testing.T) {
	logger := log.NewNopLogger()
	ctx := context.Background()

	schema, err := profile.Schema()
	require.NoError(t, err)

	fileContent, err := os.ReadFile("../query/testdata/alloc_objects.pb.gz")
	require.NoError(t, err)

	r, err := gzip.NewReader(bytes.NewReader(fileContent))
	require.NoError(t, err)

	fileContent, err = io.ReadAll(r)
	require.NoError(t, err)
	require.NoError(t, r.Close())

	table := &fakeTable{
		schema: schema,
	}
	req := &profilestorepb.WriteRawRequest{
		Series: []*profilestorepb.RawProfileSeries{{
			Labels: &profilestorepb.LabelSet{
				Labels: []*profilestorepb.Label{
					{
						Name:  "__name__",
						Value: "memory",
					},
					{
						Name:  "job",
						Value: "default",
					},
				},
			},
			Samples: []*profilestorepb.RawSample{{
				RawProfile: fileContent,
			}},
		}},
	}
	normalizedReq, err := normalizer.NormalizeWriteRawRequest(ctx, normalizer.New(), req)
	require.NoError(t, err)
	ingester := NewIngester(logger, memory.DefaultAllocator, table, schema)
	err = ingester.Ingest(ctx, normalizedReq)
	require.NoError(t, err)
	require.Equal(t, 1, len(table.inserts))
	got, err := table.inserts[0].MarshalJSON()
	require.NoError(t, err)

	want, err := os.ReadFile("testdata/ingest_uncompressed_arrow.json")
	require.NoError(t, err)
	require.JSONEq(t, string(want), string(got))
}

func BenchmarkNormalizeWriteRawRequest(b *testing.B) {
	ctx := context.Background()

	fileContent, err := os.ReadFile("../query/testdata/alloc_objects.pb.gz")
	require.NoError(b, err)

	n := normalizer.New()
	req := &profilestorepb.WriteRawRequest{
		Series: []*profilestorepb.RawProfileSeries{{
			Labels: &profilestorepb.LabelSet{
				Labels: []*profilestorepb.Label{
					{
						Name:  "__name__",
						Value: "memory",
					},
					{
						Name:  "job",
						Value: "default",
					},
				},
			},
			Samples: []*profilestorepb.RawSample{{
				RawProfile: fileContent,
			}},
		}},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err = normalizer.NormalizeWriteRawRequest(ctx, n, req)
		if err != nil {
			b.Fatal(err)
		}
	}
}