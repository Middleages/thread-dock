package statev2

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"syscall"

	contractv2 "thread-dock/internal/contract/v2"
)

var (
	ErrConflict      = errors.New("state conflict")
	ErrNotFound      = errors.New("work state not found")
	ErrBusy          = errors.New("work state is busy")
	ErrWorkBusy      = ErrBusy
	ErrStaleRevision = errors.New("stale revision")
)

type store struct {
	root string
	mu   sync.Mutex
}
type lease struct{ f *os.File }

func NewStore(root string) Store { return &store{root: filepath.Join(root, "v2", "work")} }

func (s *store) CreatePlan(ctx context.Context, snapshot WorkSnapshot, requestID contractv2.RequestID, payloadHash string) (WorkSnapshot, error) {
	if err := contextErr(ctx); err != nil {
		return WorkSnapshot{}, err
	}
	if err := validatePlan(snapshot); err != nil {
		return WorkSnapshot{}, err
	}
	canonical, err := canonicalContract(snapshot.Contract)
	if err != nil {
		return WorkSnapshot{}, err
	}
	sum := sha256.Sum256(canonical)
	if snapshot.ContractHash != hex.EncodeToString(sum[:]) {
		return WorkSnapshot{}, errors.New("contract hash mismatch")
	}
	if requestID == "" || strings.TrimSpace(payloadHash) == "" {
		return WorkSnapshot{}, errors.New("request ID and payload hash are required")
	}
	dir := s.workDir(snapshot.WorkID)
	if err := ensureDir(filepath.Dir(s.root)); err != nil {
		return WorkSnapshot{}, fmt.Errorf("create v2 directory: %w", err)
	}
	if err := ensureDir(s.root); err != nil {
		return WorkSnapshot{}, fmt.Errorf("create work directory: %w", err)
	}
	if err := ensureDir(dir); err != nil {
		return WorkSnapshot{}, fmt.Errorf("create work directory: %w", err)
	}
	if err := ensureDir(filepath.Join(dir, "contracts")); err != nil {
		return WorkSnapshot{}, fmt.Errorf("create work directory: %w", err)
	}
	l, err := acquireLease(dir)
	if err != nil {
		return WorkSnapshot{}, err
	}
	defer l.release()
	workPath := filepath.Join(dir, "work.json")
	if _, err := os.Stat(workPath); err == nil {
		existing, loadErr := s.loadVerified(ctx, snapshot.WorkID)
		if loadErr != nil {
			return WorkSnapshot{}, loadErr
		}
		if receipt, ok := existing.Receipts[requestID]; ok {
			if receipt.PayloadHash != payloadHash {
				return WorkSnapshot{}, fmt.Errorf("%w: %w", ErrConflict, &RequestConflictError{RequestID: requestID, ExistingHash: receipt.PayloadHash, PayloadHash: payloadHash})
			}
			return decodeReceiptResult(receipt, existing.Contract, existing.ContractHash)
		}
		return WorkSnapshot{}, fmt.Errorf("%w: work %q already exists", ErrConflict, snapshot.WorkID)
	} else if !errors.Is(err, os.ErrNotExist) {
		return WorkSnapshot{}, err
	}
	contractPath := filepath.Join(dir, "contracts", "1.json")
	if _, err := os.Stat(contractPath); err == nil {
		existing, readErr := readContract(contractPath)
		existingBytes, existingCanonicalErr := canonicalContract(existing)
		expectedBytes, expectedCanonicalErr := canonicalContract(snapshot.Contract)
		if readErr != nil || existingCanonicalErr != nil || expectedCanonicalErr != nil || !bytes.Equal(existingBytes, expectedBytes) {
			return WorkSnapshot{}, fmt.Errorf("%w: contract revision already exists", ErrConflict)
		}
		if err := os.Chmod(contractPath, 0600); err != nil {
			return WorkSnapshot{}, fmt.Errorf("harden contract: %w", err)
		}
	} else if errors.Is(err, os.ErrNotExist) {
		if err := writeContract(contractPath, snapshot.Contract); err != nil {
			return WorkSnapshot{}, err
		}
	} else {
		return WorkSnapshot{}, err
	}
	clientResult := clientResultProjection(snapshot)
	if snapshot.Receipts == nil {
		snapshot.Receipts = map[contractv2.RequestID]Receipt{}
	} else {
		snapshot.Receipts = cloneReceipts(snapshot.Receipts)
	}
	result, err := json.Marshal(clientResult)
	if err != nil {
		return WorkSnapshot{}, err
	}
	snapshot.Receipts[requestID] = Receipt{RequestID: requestID, PayloadHash: payloadHash, Status: "committed", Result: result}
	if err := writeSnapshot(workPath, snapshot); err != nil {
		return WorkSnapshot{}, err
	}
	return clientResult, nil
}

func (s *store) Load(ctx context.Context, id contractv2.WorkID) (WorkSnapshot, error) {
	if err := contextErr(ctx); err != nil {
		return WorkSnapshot{}, err
	}
	if err := validID(string(id)); err != nil {
		return WorkSnapshot{}, err
	}
	return s.loadVerified(ctx, id)
}

func (s *store) loadVerified(ctx context.Context, id contractv2.WorkID) (WorkSnapshot, error) {
	if err := contextErr(ctx); err != nil {
		return WorkSnapshot{}, err
	}
	f, err := os.Open(filepath.Join(s.workDir(id), "work.json"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return WorkSnapshot{}, fmt.Errorf("%w: %q", ErrNotFound, id)
		}
		return WorkSnapshot{}, err
	}
	defer f.Close()
	snapshot, err := decodeSnapshot(f)
	_ = f.Close()
	if err != nil {
		return WorkSnapshot{}, err
	}
	if err := verifyContractFile(s.workDir(id), snapshot); err != nil {
		return WorkSnapshot{}, err
	}
	return snapshot, nil
}

func (s *store) List(ctx context.Context) ([]WorkSnapshot, error) {
	if err := contextErr(ctx); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.root)
	if errors.Is(err, os.ErrNotExist) {
		return []WorkSnapshot{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list work: %w", err)
	}
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasSuffix(entry.Name(), ".tmp") || strings.HasPrefix(entry.Name(), ".tmp-") {
			continue
		}
		ids = append(ids, entry.Name())
	}
	work := make([]WorkSnapshot, 0, len(ids))
	for _, id := range ids {
		snapshot, err := s.Load(ctx, contractv2.WorkID(id))
		if err != nil {
			return nil, err
		}
		if snapshot.WorkID != contractv2.WorkID(id) {
			return nil, fmt.Errorf("work snapshot ID %q does not match directory %q", snapshot.WorkID, id)
		}
		work = append(work, snapshot)
	}
	sort.Slice(work, func(i, j int) bool { return work[i].WorkID < work[j].WorkID })
	return work, nil
}

func (s *store) Apply(ctx context.Context, request TransitionRequest) (WorkSnapshot, error) {
	if err := contextErr(ctx); err != nil {
		return WorkSnapshot{}, err
	}
	if err := validID(string(request.WorkID)); err != nil {
		return WorkSnapshot{}, err
	}
	if err := validateApplyRequest(request); err != nil {
		return WorkSnapshot{}, err
	}
	dir := s.workDir(request.WorkID)
	l, err := acquireLease(dir)
	if err != nil {
		return WorkSnapshot{}, err
	}
	defer l.release()
	f, err := os.Open(filepath.Join(dir, "work.json"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return WorkSnapshot{}, fmt.Errorf("%w: %q", ErrNotFound, request.WorkID)
		}
		return WorkSnapshot{}, err
	}
	snapshot, err := decodeSnapshot(f)
	_ = f.Close()
	if err != nil {
		return WorkSnapshot{}, err
	}
	if err := verifyContractFile(dir, snapshot); err != nil {
		return WorkSnapshot{}, err
	}
	if receipt, ok := snapshot.Receipts[request.RequestID]; ok {
		if receipt.PayloadHash == request.PayloadHash {
			return decodeReceiptResult(receipt, snapshot.Contract, snapshot.ContractHash)
		}
		return WorkSnapshot{}, fmt.Errorf("%w: %w", ErrConflict, &RequestConflictError{RequestID: request.RequestID, ExistingHash: receipt.PayloadHash, PayloadHash: request.PayloadHash})
	}
	if request.ExpectedRevision != snapshot.Revision {
		return WorkSnapshot{}, &StaleRevisionError{CurrentRevision: snapshot.Revision, CurrentState: snapshot.State}
	}
	immutable := snapshot
	immutableContract, err := canonicalContract(snapshot.Contract)
	if err != nil {
		return WorkSnapshot{}, err
	}
	immutable.Receipts = cloneReceipts(snapshot.Receipts)
	if err := applyTransition(&snapshot, request); err != nil {
		return WorkSnapshot{}, err
	}
	if snapshot.WorkID != request.WorkID || snapshot.Revision != request.ExpectedRevision ||
		snapshot.SchemaVersion != immutable.SchemaVersion || snapshot.ProjectID != immutable.ProjectID ||
		snapshot.ContractHash != immutable.ContractHash || !sameCanonicalContract(snapshot.Contract, immutableContract) ||
		!reflect.DeepEqual(snapshot.Receipts, immutable.Receipts) {
		return WorkSnapshot{}, errors.New("transition may not change work identity or revision")
	}
	snapshot.Revision++
	if err := validateSnapshot(snapshot); err != nil {
		return WorkSnapshot{}, err
	}
	clientResult := clientResultProjection(snapshot)
	result, err := json.Marshal(clientResult)
	if err != nil {
		return WorkSnapshot{}, err
	}
	if snapshot.Receipts == nil {
		snapshot.Receipts = map[contractv2.RequestID]Receipt{}
	}
	snapshot.Receipts[request.RequestID] = Receipt{RequestID: request.RequestID, PayloadHash: request.PayloadHash, Status: "committed", Result: result}
	if err := writeSnapshot(filepath.Join(dir, "work.json"), snapshot); err != nil {
		return WorkSnapshot{}, err
	}
	return clientResult, nil
}

func decodeReceiptResult(receipt Receipt, contract contractv2.WorkItemContract, currentContractHash string) (WorkSnapshot, error) {
	if receipt.Status != "committed" || len(receipt.Result) == 0 {
		return WorkSnapshot{}, errors.New("invalid request receipt")
	}
	decoder := json.NewDecoder(bytes.NewReader(receipt.Result))
	decoder.DisallowUnknownFields()
	var result WorkSnapshot
	if err := decoder.Decode(&result); err != nil {
		return WorkSnapshot{}, fmt.Errorf("decode request receipt: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return WorkSnapshot{}, errors.New("decode request receipt: trailing JSON")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(receipt.Result, &fields); err != nil {
		return WorkSnapshot{}, fmt.Errorf("decode request receipt: %w", err)
	}
	var normalizeErr error
	result, normalizeErr = normalizeFoundationSnapshot(result, fieldsPresent(fields, "taskStates"), fieldsPresent(fields, "publications"), fieldsPresent(fields, "control"))
	if normalizeErr != nil {
		return WorkSnapshot{}, normalizeErr
	}
	legacy := !fieldsPresent(fields, "taskStates") && !fieldsPresent(fields, "publications") && !fieldsPresent(fields, "control")
	if err := validateDecodedSnapshot(result, fieldsPresent(fields, "control"), legacy); err != nil {
		return WorkSnapshot{}, err
	}
	if len(result.Receipts) != 0 {
		return WorkSnapshot{}, errors.New("request receipt result must have empty receipts")
	}
	currentCanonical, err := canonicalContract(contract)
	if err != nil {
		return WorkSnapshot{}, err
	}
	resultCanonical, err := canonicalContract(result.Contract)
	if err != nil || !bytes.Equal(resultCanonical, currentCanonical) {
		return WorkSnapshot{}, errors.New("request receipt contract mismatch")
	}
	sum := sha256.Sum256(currentCanonical)
	hash := hex.EncodeToString(sum[:])
	if result.ContractHash != hash || currentContractHash != hash {
		return WorkSnapshot{}, errors.New("request receipt contract hash mismatch")
	}
	return result, nil
}

func clientResultProjection(snapshot WorkSnapshot) WorkSnapshot {
	snapshot.Receipts = map[contractv2.RequestID]Receipt{}
	return snapshot
}

func verifyContractFile(dir string, snapshot WorkSnapshot) error {
	stored, err := readContract(filepath.Join(dir, "contracts", "1.json"))
	if err != nil {
		return fmt.Errorf("read contract revision 1: %w", err)
	}
	external, err := canonicalContract(stored)
	if err != nil {
		return err
	}
	embedded, err := canonicalContract(snapshot.Contract)
	if err != nil || !bytes.Equal(external, embedded) {
		return errors.New("embedded contract does not match contract revision 1")
	}
	sum := sha256.Sum256(external)
	if snapshot.ContractHash != hex.EncodeToString(sum[:]) {
		return errors.New("contract hash mismatch")
	}
	return nil
}

func validatePlan(s WorkSnapshot) error {
	if s.SchemaVersion != 2 || s.Revision != 1 || s.State != StateAwaitingApproval {
		return errors.New("plan must be schema 2, revision 1, awaiting approval")
	}
	if err := validID(string(s.ProjectID)); err != nil || validID(string(s.WorkID)) != nil {
		return errors.New("project and work IDs are required")
	}
	if s.Contract.Version != contractv2.CurrentVersion || s.Contract.Revision != 1 || s.Contract.WorkID != s.WorkID || s.Contract.ProjectID != s.ProjectID {
		return errors.New("contract identity or revision does not match plan")
	}
	if len(contractv2.Validate(s.Contract)) > 0 {
		return errors.New("contract is invalid")
	}
	return validateSnapshot(s)
}
func validateSnapshot(s WorkSnapshot) error {
	if s.SchemaVersion != 2 || s.ProjectID == "" || s.WorkID == "" || s.Revision == 0 || s.State == "" || s.ContractHash == "" || s.Contract.Version != 2 {
		return errors.New("invalid work snapshot")
	}
	if s.Contract.WorkID != s.WorkID || s.Contract.ProjectID != s.ProjectID || s.Contract.Revision != 1 {
		return errors.New("snapshot contract mismatch")
	}
	if len(contractv2.Validate(s.Contract)) > 0 {
		return errors.New("contract is invalid")
	}
	if !validState(s.State) {
		return fmt.Errorf("unknown workflow state %q", s.State)
	}
	if s.Receipts == nil {
		return errors.New("receipts map is required")
	}
	if (s.Control.ApprovedContractHash == "") != (s.Control.ApprovalRef == "") {
		return errors.New("approval hash and reference must be provided together")
	}
	if s.Control.ApprovedContractHash != "" && s.Control.ApprovedContractHash != s.ContractHash {
		return errors.New("approved contract hash does not match contract")
	}
	return validateTaskStates(s)
}

func validateDecodedSnapshot(s WorkSnapshot, controlPresent, legacy bool) error {
	if !legacy && !controlPresent {
		return errors.New("control field is required")
	}
	return validateSnapshot(s)
}
func (s *store) workDir(id contractv2.WorkID) string { return filepath.Join(s.root, string(id)) }
func validID(v string) error {
	if v == "" || v != strings.TrimSpace(v) || strings.ContainsAny(v, "/\\:\r\n\t ") {
		return errors.New("invalid ID")
	}
	return nil
}
func decodeSnapshot(r io.Reader) (WorkSnapshot, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return WorkSnapshot{}, fmt.Errorf("decode snapshot: %w", err)
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	var s WorkSnapshot
	if err := d.Decode(&s); err != nil {
		return WorkSnapshot{}, fmt.Errorf("decode snapshot: %w", err)
	}
	var x any
	if err := d.Decode(&x); err != io.EOF {
		return WorkSnapshot{}, errors.New("decode snapshot: trailing JSON")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return WorkSnapshot{}, fmt.Errorf("decode snapshot: %w", err)
	}
	taskStatesPresent := fieldsPresent(fields, "taskStates")
	publicationsPresent := fieldsPresent(fields, "publications")
	controlPresent := fieldsPresent(fields, "control")
	s, err = normalizeFoundationSnapshot(s, taskStatesPresent, publicationsPresent, controlPresent)
	if err != nil {
		return WorkSnapshot{}, err
	}
	legacy := !taskStatesPresent && !publicationsPresent && !controlPresent
	if err := validateDecodedSnapshot(s, controlPresent, legacy); err != nil {
		return WorkSnapshot{}, err
	}
	return s, nil
}

func fieldsPresent(fields map[string]json.RawMessage, name string) bool {
	_, ok := fields[name]
	return ok
}
func writeContract(path string, c contractv2.WorkItemContract) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%w: contract revision already exists", ErrConflict)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err := f.Chmod(0600); err != nil {
		_ = f.Close()
		return err
	}
	encErr := contractv2.Write(f, c)
	if encErr == nil {
		encErr = f.Sync()
	}
	closeErr := f.Close()
	if encErr != nil {
		return encErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	return syncDir(filepath.Dir(path))
}
func writeSnapshot(path string, s WorkSnapshot) error {
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err := f.Chmod(0600); err != nil {
		_ = f.Close()
		return err
	}
	encErr := json.NewEncoder(f).Encode(s)
	if encErr == nil {
		encErr = f.Sync()
	}
	closeErr := f.Close()
	if encErr != nil {
		return encErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	return syncDir(filepath.Dir(path))
}
func acquireLease(dir string) (*lease, error) {
	f, err := os.OpenFile(filepath.Join(dir, "lease.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := f.Chmod(0600); err != nil {
		_ = f.Close()
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, ErrBusy
		}
		return nil, err
	}
	return &lease{f: f}, nil
}
func (l *lease) release() {
	if l != nil && l.f != nil {
		_ = syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
		_ = l.f.Close()
	}
}
func syncDir(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

func readContract(path string) (contractv2.WorkItemContract, error) {
	f, err := os.Open(path)
	if err != nil {
		return contractv2.WorkItemContract{}, err
	}
	defer f.Close()
	return contractv2.Read(f)
}

func canonicalContract(c contractv2.WorkItemContract) ([]byte, error) {
	var data bytes.Buffer
	if err := contractv2.Write(&data, c); err != nil {
		return nil, err
	}
	return data.Bytes(), nil
}

func sameCanonicalContract(c contractv2.WorkItemContract, expected []byte) bool {
	data, err := canonicalContract(c)
	return err == nil && bytes.Equal(data, expected)
}

func cloneReceipts(receipts map[contractv2.RequestID]Receipt) map[contractv2.RequestID]Receipt {
	clone := make(map[contractv2.RequestID]Receipt, len(receipts))
	for id, receipt := range receipts {
		receipt.Result = append(json.RawMessage(nil), receipt.Result...)
		clone[id] = receipt
	}
	return clone
}

func validState(state WorkState) bool {
	switch state {
	case StateDraft, StateAwaitingApproval, StateQueued, StateRunning, StatePaused, StateNeedsOperator, StateReadyForPR, StateReview, StatePartiallyMerged, StatePublicationPending, StateCompleted:
		return true
	default:
		return false
	}
}

func ensureDir(path string) error {
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	return os.Chmod(path, 0700)
}
func contextErr(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}
