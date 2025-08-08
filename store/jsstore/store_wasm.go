//go:build wasm

package jsstore

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"syscall/js"
	"time"

	"go.mau.fi/util/random"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/util/keys"
)

// Static assertions to ensure JSStore implements the required interfaces.
// The compiler will fail here if any methods are missing.
var _ store.DeviceContainer = (*JSStore)(nil)
var _ store.AllStores = (*JSStore)(nil)

type JSStore struct {
	jsStorage js.Value
}

// NewJSStore creates a new JSStore bridge. It will panic if the required
// `window.whatsmeowStorage` object is not found in the host JavaScript environment.
func NewJSStore() *JSStore {
	jsStorage := js.Global().Get("whatsmeowStorage")
	if !jsStorage.Truthy() {
		panic("window.whatsmeowStorage object not found. Ensure the JS storage bridge is loaded.")
	}
	return &JSStore{jsStorage: jsStorage}
}

// --- Implementation of store.DeviceContainer ---

// NewDevice creates a new device configuration for whatsmeow.
// It correctly wires up the device to use this JS bridge for all its storage needs.
func (s *JSStore) NewDevice() *store.Device {
	device := &store.Device{}

	device.NoiseKey = keys.NewKeyPair()
	device.IdentityKey = keys.NewKeyPair()
	device.SignedPreKey = device.IdentityKey.CreateSignedPreKey(1)
	device.RegistrationID = uint32(rand.Intn(16380)) + 1
	device.AdvSecretKey = random.Bytes(32)
	// Wire up all the storage interfaces to this JSStore instance.
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
	return device
}

// GetFirstDevice should fetch the primary device from the JS storage layer.
// For now, it creates a new device, but this should be expanded to load existing data.
func (s *JSStore) GetFirstDevice(ctx context.Context) (*store.Device, error) {
	// CORRECTED: Call the method on the instance 's', not the package 'store'.
	return s.NewDevice(), nil
}

// GetDevice is not yet implemented for the JS bridge.
func (s *JSStore) GetDevice(ctx context.Context, jid types.JID) (*store.Device, error) {
	return nil, errors.New("GetDevice not yet implemented for jsstore")
}

// PutDevice is not yet implemented for the JS bridge.
func (s *JSStore) PutDevice(ctx context.Context, device *store.Device) error {
	return errors.New("PutDevice not implemented")
}

// DeleteDevice is not yet implemented for the JS bridge.
func (s *JSStore) DeleteDevice(ctx context.Context, device *store.Device) error {
	return errors.New("DeleteDevice not implemented")
}

// --- Helper for awaiting JS Promises ---

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

// --- Implementation of store.AllStores (Sessions are already implemented) ---

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

// --- Stubs for the rest of store.AllStores ---

func (s *JSStore) PutIdentity(ctx context.Context, address string, key [32]byte) error {
	return errors.New("not implemented")
}
func (s *JSStore) DeleteAllIdentities(ctx context.Context, phone string) error {
	return errors.New("not implemented")
}
func (s *JSStore) DeleteIdentity(ctx context.Context, address string) error {
	return errors.New("not implemented")
}
func (s *JSStore) IsTrustedIdentity(ctx context.Context, address string, key [32]byte) (bool, error) {
	return false, errors.New("not implemented")
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
func (s *JSStore) MigratePNToLID(ctx context.Context, pn, lid types.JID) error {
	return errors.New("not implemented")
}
func (s *JSStore) GetOrGenPreKeys(ctx context.Context, count uint32) ([]*keys.PreKey, error) {
	return nil, errors.New("not implemented")
}
func (s *JSStore) GenOnePreKey(ctx context.Context) (*keys.PreKey, error) {
	return nil, errors.New("not implemented")
}
func (s *JSStore) GetPreKey(ctx context.Context, id uint32) (*keys.PreKey, error) {
	return nil, errors.New("not implemented")
}
func (s *JSStore) RemovePreKey(ctx context.Context, id uint32) error {
	return errors.New("not implemented")
}
func (s *JSStore) MarkPreKeysAsUploaded(ctx context.Context, upToID uint32) error {
	return errors.New("not implemented")
}
func (s *JSStore) UploadedPreKeyCount(ctx context.Context) (int, error) {
	return 0, errors.New("not implemented")
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
func (s *JSStore) PutAppStateVersion(ctx context.Context, name string, version uint64, hash [128]byte) error {
	return errors.New("not implemented")
}
func (s *JSStore) GetAppStateVersion(ctx context.Context, name string) (uint64, [128]byte, error) {
	return 0, [128]byte{}, errors.New("not implemented")
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
func (s *JSStore) PutPushName(ctx context.Context, user types.JID, pushName string) (bool, string, error) {
	return false, "", errors.New("not implemented")
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
func (s *JSStore) GetLIDForPN(ctx context.Context, pn types.JID) (types.JID, error) {
	return types.JID{}, errors.New("not implemented")
}
func (s *JSStore) GetPNForLID(ctx context.Context, lid types.JID) (types.JID, error) {
	return types.JID{}, errors.New("not implemented")
}
func (s *JSStore) PutManyLIDMappings(ctx context.Context, mappings []store.LIDMapping) error {
	return errors.New("not implemented")
}
func (s *JSStore) PutLIDMapping(ctx context.Context, lid types.JID, jid types.JID) error {
	return errors.New("not implemented")
}
