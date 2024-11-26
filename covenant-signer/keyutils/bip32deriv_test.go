package keyutils

import (
	"testing"

	"github.com/stretchr/testify/require"
)

var parsePathTests = []struct {
	name     string
	path     string
	expected []uint32
	wantErr  bool
}{
	{
		name:     "valid hardened and non-hardened path",
		path:     "84h/1h/0h/0/0",
		expected: []uint32{0x80000054, 0x80000001, 0x80000000, 0, 0},
		wantErr:  false,
	},
}

func TestParsePath(t *testing.T) {
	for _, tt := range parsePathTests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePath(tt.path)

			if tt.wantErr {
				require.Error(t, err)
				return
			} else {
				require.NoError(t, err)
			}

			if len(got) != len(tt.expected) {
				t.Errorf("ParsePath() got length = %v, want length = %v", len(got), len(tt.expected))
				return
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("ParsePath() got[%d] = %v, want[%d] = %v", i, got[i], i, tt.expected[i])
				}
			}
		})
	}
}

var deriveKeyTests = []struct {
	name              string
	masterPrivKey     string
	derivationPath    string
	expectedPublicKey string
}{
	{
		name:              "derive from master at index 0",
		masterPrivKey:     "tprv8ZgxMBicQKsPdNsenC9sBio2D65FssTr2hz7eeeLqTZbvYe8v6BbTjzctbhhSTtFu1QCFMC1Ag58cokSC9q8E68w71nZDqkFnxNdeAXVsG9",
		derivationPath:    "84h/1h/0h/0/0",
		expectedPublicKey: "026f1738cc40f1a67b0726d8a3184277a1137422cfdb0d888a0dfdf69b907f8840",
	},
	{
		name:              "derive from master at index 0 from new private key",
		masterPrivKey:     "tprv8ZgxMBicQKsPecqD8mUyo6R18nZE2KyUJGudL3gFsYApS6wYd35sYw6wS2rWHuFakSP6Z9k4NgmP93JvJGbwf4Cc1o7bQWcsUgR4mELA91q",
		derivationPath:    "84h/1h/0h/0/0",
		expectedPublicKey: "02a81acd3457a7f622ab8c5800f0afd21a58a0dc2f35cefb1c623bc0033b012554",
	},
	{
		name:              "derive from master at index 7",
		masterPrivKey:     "tprv8ZgxMBicQKsPecqD8mUyo6R18nZE2KyUJGudL3gFsYApS6wYd35sYw6wS2rWHuFakSP6Z9k4NgmP93JvJGbwf4Cc1o7bQWcsUgR4mELA91q",
		derivationPath:    "84h/1h/0h/0/7",
		expectedPublicKey: "030c1362c11495d10247ad916db47504a0d4704fae2951b9bdc460edbd3c4df54b",
	},
}

func TestDeriveKey(t *testing.T) {
	for _, tt := range deriveKeyTests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DeriveChildKey(tt.masterPrivKey, tt.derivationPath)
			require.NoError(t, err)
			require.Equal(t, tt.expectedPublicKey, got.PublicKey)
		})
	}
}
