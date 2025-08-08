//go:build wasm

package jsstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"syscall/js"
	"time"

	"go.mau.fi/libsignal/protocol"
	"go.mau.fi/util/random"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/util/keys"
)

const singleDeviceJID = "my-device"

type serializableDevice struct {
	NoiseKey       []byte `json:"noiseKey"`
	IdentityKey    []byte `json:"identityKey"`
	SignedPreKey   []byte `json:"signedPreKey"`
	SignedPreKeyID uint32 `json:"signedPreKeyId"`
	RegistrationID uint32 `json:"registrationId"`
	AdvSecretKey   []byte `json:"advSecretKey"`
	JID            string `json:"jid"`
}
type serializablePreKey struct {
	ID         uint32   `json:"id"`
	PrivateKey [32]byte `json:"privateKey"`
}
type serializableAppStateVersion struct {
	Version uint64 `json:"version"`
	Hash    []byte `json:"hash"`
}

var _ store.DeviceContainer = (*JSStore)(nil)
var _ store.AllStores = (*JSStore)(nil)

type JSStore struct {
	jsStorage js.Value
}

func NewJSStore() *JSStore {
	jsStorage := js.Global().Get("whatsmeowStorage")
	if !jsStorage.Truthy() {
		panic("window.whatsmeowStorage object not found. Ensure the JS storage bridge is loaded.")
	}
	return &JSStore{jsStorage: jsStorage}
}

func (s *JSStore) NewDevice() *store.Device {
	device := &store.Device{}
	s.wireUpStores(device)

	device.NoiseKey = keys.NewKeyPair()
	device.IdentityKey = keys.NewKeyPair()
	device.SignedPreKey = device.IdentityKey.CreateSignedPreKey(1)
	device.RegistrationID = uint32(rand.Intn(16380)) + 1
	device.AdvSecretKey = random.Bytes(32)

	return device
}

func (s *JSStore) GetFirstDevice(ctx context.Context) (*store.Device, error) {
	promise := s.jsStorage.Call("getDevice", singleDeviceJID)
	result, err := awaitPromise(promise)
	if err != nil {
		return nil, fmt.Errorf("js getDevice failed: %w", err)
	}

	if !result.Truthy() {
		return s.NewDevice(), nil
	}

	jsonData := result.String()
	var saved serializableDevice
	if err := json.Unmarshal([]byte(jsonData), &saved); err != nil {
		return nil, fmt.Errorf("failed to unmarshal device JSON: %w", err)
	}

	jid, err := types.ParseJID(saved.JID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse JID from saved device: %w", err)
	}

	device := &store.Device{
		ID:             &jid,
		NoiseKey:       keys.NewKeyPairFromPrivateKey(as32Byte(saved.NoiseKey)),
		IdentityKey:    keys.NewKeyPairFromPrivateKey(as32Byte(saved.IdentityKey)),
		SignedPreKey:   keys.NewKeyPairFromPrivateKey(as32Byte(saved.SignedPreKey)).CreateSignedPreKey(saved.SignedPreKeyID),
		RegistrationID: saved.RegistrationID,
		AdvSecretKey:   saved.AdvSecretKey,
	}

	s.wireUpStores(device)
	return device, nil
}

func (s *JSStore) GetDevice(ctx context.Context, jid types.JID) (*store.Device, error) {
	return nil, errors.New("GetDevice not yet implemented for jsstore")
}

func (s *JSStore) PutDevice(ctx context.Context, device *store.Device) error {
	if device.ID == nil {
		return errors.New("cannot put device with nil JID")
	}

	saved := serializableDevice{
		NoiseKey:       (*device.NoiseKey.Priv)[:],
		IdentityKey:    (*device.IdentityKey.Priv)[:],
		SignedPreKey:   (*device.SignedPreKey.Priv)[:],
		SignedPreKeyID: device.SignedPreKey.KeyID,
		RegistrationID: device.RegistrationID,
		AdvSecretKey:   device.AdvSecretKey,
		JID:            device.ID.String(),
	}

	jsonData, err := json.Marshal(saved)
	if err != nil {
		return fmt.Errorf("failed to marshal device to JSON: %w", err)
	}

	promise := s.jsStorage.Call("putDevice", singleDeviceJID, string(jsonData))
	_, err = awaitPromise(promise)
	return err
}

func (s *JSStore) wireUpStores(device *store.Device) {
	device.Container = s
	device.Identities = s
	device.Sessions = s
	device.PreKeys = s
	device.SenderKeys = s
	device.AppStateKeys = s
	device.AppState = s
	device.Contacts = s
	device.ChatSettings = s
	device.MsgSecrets = s
	device.PrivacyTokens = s
	device.EventBuffer = s
	device.LIDs = s
}

func (s *JSStore) DeleteDevice(ctx context.Context, device *store.Device) error {
	return errors.New("DeleteDevice not implemented")
}

func awaitPromise(promise js.Value) (js.Value, error) {
	if promise.IsUndefined() || promise.IsNull() {
		return js.Undefined(), errors.New("promise is nil or undefined")
	}
	resultChan := make(chan js.Value)
	errorChan := make(chan error)
	successCallback := js.FuncOf(func(this js.Value, args []js.Value) any {
		resultChan <- args[0]
		return nil
	})
	defer successCallback.Release()
	errorCallback := js.FuncOf(func(this js.Value, args []js.Value) any {
		jsErr := args[0]
		goErr := fmt.Errorf("JS promise rejected: %s", jsErr.Call("toString").String())
		errorChan <- goErr
		return nil
	})
	defer errorCallback.Release()
	promise.Call("then", successCallback).Call("catch", errorCallback)
	select {
	case result := <-resultChan:
		return result, nil
	case err := <-errorChan:
		return js.Undefined(), err
	}
}

func (s *JSStore) PutSession(ctx context.Context, address string, session []byte) error {
	jsMethod := s.jsStorage.Get("putSession")
	if !jsMethod.Truthy() {
		return errors.New("JavaScript method 'putSession' not found")
	}
	jsBuffer := js.Global().Get("Uint8Array").New(len(session))
	js.CopyBytesToJS(jsBuffer, session)
	promise := jsMethod.Invoke(address, jsBuffer)
	_, err := awaitPromise(promise)
	if err != nil {
		return fmt.Errorf("js putSession failed: %w", err)
	}
	return nil
}

func (s *JSStore) GetSession(ctx context.Context, address string) ([]byte, error) {
	jsMethod := s.jsStorage.Get("getSession")
	if !jsMethod.Truthy() {
		return nil, errors.New("JavaScript method 'getSession' not found")
	}
	promise := jsMethod.Invoke(address)
	result, err := awaitPromise(promise)
	if err != nil {
		return nil, fmt.Errorf("js getSession failed: %w", err)
	}
	if result.IsNull() || result.IsUndefined() {
		return nil, nil
	}
	goBytes := make([]byte, result.Get("length").Int())
	js.CopyBytesToGo(goBytes, result)
	return goBytes, nil
}

func as32Byte(b []byte) [32]byte {
	var a [32]byte
	copy(a[:], b)
	return a
}

func (s *JSStore) PutIdentity(ctx context.Context, address string, key [32]byte) error {
	jsBuffer := js.Global().Get("Uint8Array").New(len(key))
	js.CopyBytesToJS(jsBuffer, key[:])
	promise := s.jsStorage.Call("putIdentity", address, jsBuffer)
	_, err := awaitPromise(promise)
	return err
}

func (s *JSStore) IsTrustedIdentity(ctx context.Context, address string, key [32]byte) (bool, error) {
	promise := s.jsStorage.Call("getIdentity", address)
	result, err := awaitPromise(promise)
	if err != nil {
		return false, err
	}
	if !result.Truthy() {
		return true, nil
	}
	existingKey := make([]byte, result.Get("length").Int())
	js.CopyBytesToGo(existingKey, result)
	return [32]byte(existingKey) == key, nil
}

func (s *JSStore) PutLIDMapping(ctx context.Context, lid types.JID, pn types.JID) error {
	promise := s.jsStorage.Call("putLIDMapping", lid.String(), pn.String())
	_, err := awaitPromise(promise)
	return err
}

func (s *JSStore) GetLIDForPN(ctx context.Context, pn types.JID) (types.JID, error) {
	promise := s.jsStorage.Call("getLIDForPN", pn.String())
	result, err := awaitPromise(promise)
	if err != nil || !result.Truthy() {
		return types.JID{}, err
	}
	return types.ParseJID(result.String())
}

func (s *JSStore) GetPNForLID(ctx context.Context, lid types.JID) (types.JID, error) {
	promise := s.jsStorage.Call("getPNForLID", lid.String())
	result, err := awaitPromise(promise)
	if err != nil || !result.Truthy() {
		return types.JID{}, err
	}
	return types.ParseJID(result.String())
}

func (s *JSStore) GetPreKey(ctx context.Context, id uint32) (*keys.PreKey, error) {
	promise := s.jsStorage.Call("getPreKey", id)
	result, err := awaitPromise(promise)
	if err != nil {
		return nil, fmt.Errorf("js getPreKey failed: %w", err)
	}
	if !result.Truthy() {
		return nil, nil
	}
	var saved serializablePreKey
	if err := json.Unmarshal([]byte(result.String()), &saved); err != nil {
		return nil, fmt.Errorf("failed to unmarshal prekey: %w", err)
	}
	return keys.NewKeyPairFromPrivateKey(saved.PrivateKey).CreateSignedPreKey(saved.ID), nil
}

func (s *JSStore) PutPreKey(id uint32, priv [32]byte) error {
	saved := serializablePreKey{ID: id, PrivateKey: priv}
	jsonData, err := json.Marshal(saved)
	if err != nil {
		return fmt.Errorf("failed to marshal prekey: %w", err)
	}
	promise := s.jsStorage.Call("putPreKey", id, string(jsonData))
	_, err = awaitPromise(promise)
	return err
}

func (s *JSStore) GetOrGenPreKeys(ctx context.Context, count uint32) ([]*keys.PreKey, error) {
	promise := s.jsStorage.Call("getHighestPreKeyID")
	result, err := awaitPromise(promise)
	if err != nil {
		return nil, fmt.Errorf("js getHighestPreKeyID failed: %w", err)
	}
	nextID := uint32(result.Int()) + 1

	var preKeys []*keys.PreKey
	for i := uint32(0); i < count; i++ {
		keyID := nextID + i
		kp := keys.NewKeyPair()
		if err := s.PutPreKey(keyID, *kp.Priv); err != nil {
			return nil, fmt.Errorf("failed to store generated prekey %d: %w", keyID, err)
		}
		preKeys = append(preKeys, kp.CreateSignedPreKey(keyID))
	}
	return preKeys, nil
}

func (s *JSStore) GenOnePreKey(ctx context.Context) (*keys.PreKey, error) {
	kp := keys.NewKeyPair()
	newID := uint32(time.Now().Unix())
	if err := s.PutPreKey(newID, *kp.Priv); err != nil {
		return nil, err
	}
	return kp.CreateSignedPreKey(newID), nil
}

func (s *JSStore) RemovePreKey(ctx context.Context, id uint32) error {
	promise := s.jsStorage.Call("removePreKey", id)
	_, err := awaitPromise(promise)
	return err
}

func (s *JSStore) UploadedPreKeyCount(ctx context.Context) (int, error) { return 0, nil }

func (s *JSStore) MarkPreKeysAsUploaded(ctx context.Context, upToID uint32) error {
	return nil
}

func (s *JSStore) PutAppStateVersion(ctx context.Context, name string, version uint64, hash [128]byte) error {
	data := serializableAppStateVersion{Version: version, Hash: hash[:]}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	promise := s.jsStorage.Call("putAppStateVersion", name, string(jsonData))
	_, err = awaitPromise(promise)
	return err
}

func (s *JSStore) GetAppStateVersion(ctx context.Context, name string) (uint64, [128]byte, error) {
	promise := s.jsStorage.Call("getAppStateVersion", name)
	result, err := awaitPromise(promise)
	if err != nil {
		return 0, [128]byte{}, err
	}
	if !result.Truthy() {
		return 0, [128]byte{}, nil // Not found is a valid initial state
	}
	var data serializableAppStateVersion
	if err := json.Unmarshal([]byte(result.String()), &data); err != nil {
		return 0, [128]byte{}, err
	}
	var hash [128]byte
	copy(hash[:], data.Hash)
	return data.Version, hash, nil
}

// --- ContactStore ---
func (s *JSStore) PutPushName(ctx context.Context, user types.JID, pushName string) (bool, string, error) {
	promise := s.jsStorage.Call("putPushName", user.String(), pushName)
	_, err := awaitPromise(promise)
	return true, "", err // Return true to indicate a change was made
}

// --- SessionStore ---
func (s *JSStore) MigratePNToLID(ctx context.Context, pn, lid types.JID) error {
	pnStr := protocol.NewSignalAddress(pn.User, uint32(pn.Device)).String()
	lidStr := protocol.NewSignalAddress(lid.User, uint32(lid.Device)).String()

	// Get PN session
	sessionBytes, err := s.GetSession(ctx, pnStr)
	if err != nil {
		return err
	}
	if len(sessionBytes) == 0 {
		return nil // No session to migrate
	}

	// Put it at the LID address and delete the PN address
	if err := s.PutSession(ctx, lidStr, sessionBytes); err != nil {
		return err
	}
	return s.DeleteSession(ctx, pnStr)
}

func (s *JSStore) DeleteAllIdentities(ctx context.Context, phone string) error {
	return errors.New("not implemented")
}
func (s *JSStore) DeleteIdentity(ctx context.Context, address string) error {
	return errors.New("not implemented")
}
func (s *JSStore) HasSession(ctx context.Context, address string) (bool, error) {
	return false, errors.New("not implemented")
}
func (s *JSStore) DeleteAllSessions(ctx context.Context, phone string) error {
	return errors.New("not implemented")
}
func (s *JSStore) DeleteSession(ctx context.Context, address string) error {
	return errors.New("not implemented")
}
func (s *JSStore) PutSenderKey(ctx context.Context, group, user string, session []byte) error {
	return errors.New("not implemented")
}
func (s *JSStore) GetSenderKey(ctx context.Context, group, user string) ([]byte, error) {
	return nil, errors.New("not implemented")
}
func (s *JSStore) PutAppStateSyncKey(ctx context.Context, id []byte, key store.AppStateSyncKey) error {
	return errors.New("not implemented")
}
func (s *JSStore) GetAppStateSyncKey(ctx context.Context, id []byte) (*store.AppStateSyncKey, error) {
	return nil, errors.New("not implemented")
}
func (s *JSStore) GetLatestAppStateSyncKeyID(ctx context.Context) ([]byte, error) {
	return nil, errors.New("not implemented")
}
func (s *JSStore) DeleteAppStateVersion(ctx context.Context, name string) error {
	return errors.New("not implemented")
}
func (s *JSStore) PutAppStateMutationMACs(ctx context.Context, name string, version uint64, mutations []store.AppStateMutationMAC) error {
	return errors.New("not implemented")
}
func (s *JSStore) DeleteAppStateMutationMACs(ctx context.Context, name string, indexMACs [][]byte) error {
	return errors.New("not implemented")
}
func (s *JSStore) GetAppStateMutationMAC(ctx context.Context, name string, indexMAC []byte) (valueMAC []byte, err error) {
	return nil, errors.New("not implemented")
}
func (s *JSStore) PutBusinessName(ctx context.Context, user types.JID, businessName string) (bool, string, error) {
	return false, "", errors.New("not implemented")
}
func (s *JSStore) PutContactName(ctx context.Context, user types.JID, fullName, firstName string) error {
	return errors.New("not implemented")
}
func (s *JSStore) PutAllContactNames(ctx context.Context, contacts []store.ContactEntry) error {
	return errors.New("not implemented")
}
func (s *JSStore) GetContact(ctx context.Context, user types.JID) (types.ContactInfo, error) {
	return types.ContactInfo{}, errors.New("not implemented")
}
func (s *JSStore) GetAllContacts(ctx context.Context) (map[types.JID]types.ContactInfo, error) {
	return nil, errors.New("not implemented")
}
func (s *JSStore) PutMutedUntil(ctx context.Context, chat types.JID, mutedUntil time.Time) error {
	return errors.New("not implemented")
}
func (s *JSStore) PutPinned(ctx context.Context, chat types.JID, pinned bool) error {
	return errors.New("not implemented")
}
func (s *JSStore) PutArchived(ctx context.Context, chat types.JID, archived bool) error {
	return errors.New("not implemented")
}
func (s *JSStore) GetChatSettings(ctx context.Context, chat types.JID) (types.LocalChatSettings, error) {
	return types.LocalChatSettings{}, errors.New("not implemented")
}
func (s *JSStore) PutMessageSecrets(ctx context.Context, inserts []store.MessageSecretInsert) error {
	return errors.New("not implemented")
}
func (s *JSStore) PutMessageSecret(ctx context.Context, chat, sender types.JID, id types.MessageID, secret []byte) error {
	return errors.New("not implemented")
}
func (s *JSStore) GetMessageSecret(ctx context.Context, chat, sender types.JID, id types.MessageID) ([]byte, types.JID, error) {
	return nil, types.JID{}, errors.New("not implemented")
}
func (s *JSStore) PutPrivacyTokens(ctx context.Context, tokens ...store.PrivacyToken) error {
	return errors.New("not implemented")
}
func (s *JSStore) GetPrivacyToken(ctx context.Context, user types.JID) (*store.PrivacyToken, error) {
	return nil, errors.New("not implemented")
}
func (s *JSStore) GetBufferedEvent(ctx context.Context, ciphertextHash [32]byte) (*store.BufferedEvent, error) {
	return nil, errors.New("not implemented")
}
func (s *JSStore) PutBufferedEvent(ctx context.Context, ciphertextHash [32]byte, plaintext []byte, serverTimestamp time.Time) error {
	return errors.New("not implemented")
}
func (s *JSStore) DoDecryptionTxn(ctx context.Context, fn func(context.Context) error) error {
	return errors.New("not implemented")
}
func (s *JSStore) ClearBufferedEventPlaintext(ctx context.Context, ciphertextHash [32]byte) error {
	return errors.New("not implemented")
}
func (s *JSStore) DeleteOldBufferedHashes(ctx context.Context) error {
	return errors.New("not implemented")
}
func (s *JSStore) PutManyLIDMappings(ctx context.Context, mappings []store.LIDMapping) error {
	return errors.New("not implemented")
}
