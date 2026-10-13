package backup

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"

	"golang.org/x/crypto/pbkdf2"
)

// The archive is a regular ZIP file whose entries are encrypted with WinZip
// AES-256 (AE-2). That keeps downloaded backups openable with standard tools
// such as 7-Zip, WinZip or Keka using the backup passphrase.
//
// Per entry: PBKDF2-HMAC-SHA1(passphrase, salt, 1000) derives an AES key, an
// HMAC-SHA1 key and a 2-byte password verifier. Data is (optionally) deflated,
// then encrypted with AES-CTR (little-endian counter starting at 1), then
// authenticated with HMAC-SHA1 over the ciphertext (first 10 bytes stored).

const (
	methodAES      = 99
	aesExtraID     = 0x9901
	aesKeyLen      = 32 // AES-256
	aesSaltLen     = 16
	aesVerifierLen = 2
	aesAuthLen     = 10
	pbkdf2Iter     = 1000
	maxEntrySize   = 1 << 30 // guard against zip bombs when reading uploads
	zipVersionAES  = 51
)

// ErrWrongPassphrase is returned when an entry cannot be decrypted with the
// supplied passphrase.
var ErrWrongPassphrase = errors.New("wrong backup passphrase")

// ErrCorrupt is returned when an entry fails authentication or is malformed.
var ErrCorrupt = errors.New("backup archive is corrupt or has been tampered with")

func deriveKeys(passphrase []byte, salt []byte, keyLen int) (encKey, macKey, verifier []byte) {
	dk := pbkdf2.Key(passphrase, salt, pbkdf2Iter, 2*keyLen+aesVerifierLen, sha1.New)
	return dk[:keyLen], dk[keyLen : 2*keyLen], dk[2*keyLen:]
}

// xorCTR applies WinZip's AES-CTR keystream to data in place.
func xorCTR(block cipher.Block, data []byte) {
	var ctr, ks [aes.BlockSize]byte
	ctr[0] = 1
	for off := 0; off < len(data); off += aes.BlockSize {
		block.Encrypt(ks[:], ctr[:])
		end := min(off+aes.BlockSize, len(data))
		subtle.XORBytes(data[off:end], data[off:end], ks[:end-off])
		for i := range ctr {
			ctr[i]++
			if ctr[i] != 0 {
				break
			}
		}
	}
}

// ── Writer ───────────────────────────────────────────────────────────────────

// Writer writes an AES-encrypted ZIP archive.
type Writer struct {
	zw         *zip.Writer
	passphrase []byte
}

// NewWriter returns a Writer that encrypts every entry with passphrase.
func NewWriter(w io.Writer, passphrase string) *Writer {
	return &Writer{zw: zip.NewWriter(w), passphrase: []byte(passphrase)}
}

// WriteFile adds an encrypted entry. When compress is false the data is
// stored as-is (useful for already-compressed images).
func (w *Writer) WriteFile(name string, data []byte, modified time.Time, compress bool) error {
	var payload []byte
	method := uint16(zip.Store)
	if compress {
		var buf bytes.Buffer
		fw, _ := flate.NewWriter(&buf, flate.DefaultCompression)
		if _, err := fw.Write(data); err != nil {
			return err
		}
		if err := fw.Close(); err != nil {
			return err
		}
		payload = buf.Bytes()
		method = zip.Deflate
	} else {
		payload = append([]byte(nil), data...)
	}

	salt := make([]byte, aesSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	encKey, macKey, verifier := deriveKeys(w.passphrase, salt, aesKeyLen)
	block, err := aes.NewCipher(encKey)
	if err != nil {
		return err
	}
	xorCTR(block, payload)
	mac := hmac.New(sha1.New, macKey)
	mac.Write(payload)
	auth := mac.Sum(nil)[:aesAuthLen]

	extra := make([]byte, 11)
	binary.LittleEndian.PutUint16(extra[0:], aesExtraID)
	binary.LittleEndian.PutUint16(extra[2:], 7)
	binary.LittleEndian.PutUint16(extra[4:], 2) // AE-2: CRC is not stored
	extra[6], extra[7] = 'A', 'E'
	extra[8] = 3 // AES-256
	binary.LittleEndian.PutUint16(extra[9:], method)

	fh := &zip.FileHeader{
		Name:               name,
		Method:             methodAES,
		Flags:              0x1, // encrypted
		CreatorVersion:     zipVersionAES,
		ReaderVersion:      zipVersionAES,
		Extra:              extra,
		CompressedSize64:   uint64(aesSaltLen + aesVerifierLen + len(payload) + aesAuthLen),
		UncompressedSize64: uint64(len(data)),
	}
	fh.SetModTime(modified) //nolint:staticcheck // CreateRaw needs the MS-DOS fields
	out, err := w.zw.CreateRaw(fh)
	if err != nil {
		return err
	}
	for _, part := range [][]byte{salt, verifier, payload, auth} {
		if _, err := out.Write(part); err != nil {
			return err
		}
	}
	return nil
}

// Close finishes the archive.
func (w *Writer) Close() error { return w.zw.Close() }

// ── Reader ───────────────────────────────────────────────────────────────────

// Reader reads an AES-encrypted ZIP archive.
type Reader struct {
	files      map[string]*zip.File
	names      []string
	passphrase []byte
}

// NewReader opens an archive. It does not check the passphrase; use ReadFile.
func NewReader(r io.ReaderAt, size int64, passphrase string) (*Reader, error) {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, fmt.Errorf("not a valid backup archive: %w", err)
	}
	rd := &Reader{files: map[string]*zip.File{}, passphrase: []byte(passphrase)}
	for _, f := range zr.File {
		rd.files[f.Name] = f
		rd.names = append(rd.names, f.Name)
	}
	return rd, nil
}

// Names returns all entry names in archive order.
func (r *Reader) Names() []string { return r.names }

// Has reports whether the archive contains name.
func (r *Reader) Has(name string) bool { _, ok := r.files[name]; return ok }

type aesParams struct {
	keyLen int
	method uint16
}

func parseAESExtra(extra []byte) (aesParams, bool) {
	for len(extra) >= 4 {
		id := binary.LittleEndian.Uint16(extra[0:])
		n := int(binary.LittleEndian.Uint16(extra[2:]))
		if len(extra) < 4+n {
			break
		}
		body := extra[4 : 4+n]
		if id == aesExtraID && n >= 7 && body[2] == 'A' && body[3] == 'E' {
			var kl int
			switch body[4] {
			case 1:
				kl = 16
			case 2:
				kl = 24
			case 3:
				kl = 32
			default:
				return aesParams{}, false
			}
			return aesParams{keyLen: kl, method: binary.LittleEndian.Uint16(body[5:])}, true
		}
		extra = extra[4+n:]
	}
	return aesParams{}, false
}

// ReadFile decrypts, authenticates and decompresses the named entry.
func (r *Reader) ReadFile(name string) ([]byte, error) {
	f, ok := r.files[name]
	if !ok {
		return nil, fmt.Errorf("backup archive is missing %s", name)
	}
	if f.Method != methodAES || f.Flags&0x1 == 0 {
		return nil, fmt.Errorf("%s is not encrypted with AES; refusing to restore an unencrypted backup", name)
	}
	p, ok := parseAESExtra(f.Extra)
	if !ok {
		return nil, ErrCorrupt
	}
	if f.CompressedSize64 > maxEntrySize || f.UncompressedSize64 > maxEntrySize {
		return nil, fmt.Errorf("%s is too large", name)
	}
	saltLen := p.keyLen / 2 // 8, 12 or 16 bytes
	overhead := uint64(saltLen + aesVerifierLen + aesAuthLen)
	if f.CompressedSize64 < overhead {
		return nil, ErrCorrupt
	}
	rawR, err := f.OpenRaw()
	if err != nil {
		return nil, err
	}
	raw := make([]byte, f.CompressedSize64)
	if _, err := io.ReadFull(rawR, raw); err != nil {
		return nil, ErrCorrupt
	}
	salt := raw[:saltLen]
	pv := raw[saltLen : saltLen+aesVerifierLen]
	payload := raw[saltLen+aesVerifierLen : len(raw)-aesAuthLen]
	auth := raw[len(raw)-aesAuthLen:]

	encKey, macKey, verifier := deriveKeys(r.passphrase, salt, p.keyLen)
	if subtle.ConstantTimeCompare(pv, verifier) != 1 {
		return nil, ErrWrongPassphrase
	}
	mac := hmac.New(sha1.New, macKey)
	mac.Write(payload)
	if !hmac.Equal(mac.Sum(nil)[:aesAuthLen], auth) {
		// The 2-byte verifier passes for ~1/65536 wrong passphrases; the MAC
		// catches those as well as corruption.
		return nil, ErrCorrupt
	}
	block, err := aes.NewCipher(encKey)
	if err != nil {
		return nil, err
	}
	xorCTR(block, payload)

	switch p.method {
	case zip.Store:
		return payload, nil
	case zip.Deflate:
		fr := flate.NewReader(bytes.NewReader(payload))
		defer fr.Close()
		out, err := io.ReadAll(io.LimitReader(fr, maxEntrySize+1))
		if err != nil {
			return nil, ErrCorrupt
		}
		if len(out) > maxEntrySize {
			return nil, fmt.Errorf("%s is too large", name)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("%s uses unsupported compression method %d", name, p.method)
	}
}
