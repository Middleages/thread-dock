package contract_test

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"thread-dock/internal/contract"
	"thread-dock/internal/testfixture"
)

func TestReadRejectsUnknownTopLevelField(t *testing.T) {
	_, err := contract.Read(strings.NewReader(`{"version":1,"unexpected":true}`))
	if err == nil || !strings.Contains(err.Error(), "unexpected") {
		t.Fatalf("err = %v", err)
	}
}

func TestWriteReadRoundTrip(t *testing.T) {
	want := testfixture.ValidContract()
	var buf bytes.Buffer
	if err := contract.Write(&buf, want); err != nil {
		t.Fatal(err)
	}
	got, err := contract.Read(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("want=%#v got=%#v", want, got)
	}
}

func TestWriteUsesIndentedJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := contract.Write(&buf, testfixture.ValidContract()); err != nil {
		t.Fatal(err)
	}
	var decoded any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "\n  \"version\": 1") {
		t.Fatalf("JSON is not indented: %q", buf.String())
	}
}

func TestReadRejectsInvalidContract(t *testing.T) {
	c := testfixture.ValidContract()
	c.Tasks[1].AllowedPaths = []string{"src/payments/**"}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(c); err != nil {
		t.Fatal(err)
	}
	if _, err := contract.Read(&buf); err == nil || !strings.Contains(err.Error(), "path_overlap") {
		t.Fatalf("err = %v", err)
	}
}

func TestReadRejectsTrailingJSONDocument(t *testing.T) {
	data := mustValidJSON(t)
	data = append(data, '\n', '{', '}')
	if _, err := contract.Read(bytes.NewReader(data)); err == nil || !strings.Contains(err.Error(), "JSON") {
		t.Fatalf("err = %v", err)
	}
}

func TestReadRejectsTrailingNonWhitespace(t *testing.T) {
	data := append(mustValidJSON(t), '\n', 'x')
	if _, err := contract.Read(bytes.NewReader(data)); err == nil || !strings.Contains(err.Error(), "JSON") {
		t.Fatalf("err = %v", err)
	}
}

func TestReadAcceptsTrailingWhitespace(t *testing.T) {
	data := append(mustValidJSON(t), ' ', '\n', '\t')
	if _, err := contract.Read(bytes.NewReader(data)); err != nil {
		t.Fatalf("err = %v", err)
	}
}

func mustValidJSON(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := contract.Write(&buf, testfixture.ValidContract()); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestValidFixtureMatchesCanonicalContract(t *testing.T) {
	data, err := os.ReadFile("../../testdata/contracts/valid.json")
	if err != nil {
		t.Fatal(err)
	}
	got, err := contract.Read(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(testfixture.ValidContract(), got) {
		t.Fatalf("fixture contract differs from canonical fixture: %#v", got)
	}
	var encoded bytes.Buffer
	if err := contract.Write(&encoded, testfixture.ValidContract()); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, encoded.Bytes()) {
		t.Fatalf("fixture drifted from canonical serialization")
	}
}

func TestInvalidOverlapFixtureIsRejected(t *testing.T) {
	data, err := os.ReadFile("../../testdata/contracts/invalid-overlap.json")
	if err != nil {
		t.Fatal(err)
	}
	_, err = contract.Read(bytes.NewReader(data))
	if err == nil || !strings.Contains(err.Error(), "path_overlap") {
		t.Fatalf("err = %v", err)
	}
}
