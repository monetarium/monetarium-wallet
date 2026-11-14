// Copyright (c) 2026 The Monetarium developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wallet

import (
	"context"
	"encoding/json"
	"net/http"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/decred/vspd/types/v3"
	"github.com/monetarium/monetarium-node/chaincfg/chainhash"
	"github.com/monetarium/monetarium-node/wire"
	"github.com/monetarium/monetarium-wallet/errors"
	"github.com/monetarium/monetarium-wallet/wallet/udb"
	"github.com/monetarium/monetarium-wallet/wallet/walletdb"
)

// TestVSPProcessDropsCallerFeeTxOnFailure checks that when Process cannot get
// the fee address or make the fee tx, it returns the error and does not leave
// the purchase's fee tx for the scheduled attempt: the purchase unlocks those
// inputs once Process fails and may spend them, and a failed fee tx may have
// outputs but no signatures.  A scheduled attempt that runs while Process waits
// for it owns the fee status: a fee tx it made and paid stays paid, and one
// the VSP rejected stays errored without the purchase's tx taking its place.
func TestVSPProcessDropsCallerFeeTxOnFailure(t *testing.T) {
	tests := []struct {
		name       string
		feeAddress bool // the VSP quotes a fee
		// attempt, if set, is what a scheduled attempt running while
		// Process waits for it does: "pays" makes and pays a fee tx,
		// "rejected" drops a fee tx the VSP refused.
		attempt string
	}{
		{name: "fee address fails"},
		{name: "fee tx fails", feeAddress: true},
		{name: "scheduled attempt paid while Process waited", attempt: "pays"},
		{name: "scheduled attempt's fee tx rejected while Process waited", attempt: "rejected"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			var ticket *VSPTicket
			c := testVSPClient(ctx, t, func(path string, body []byte) (int, any) {
				if !tc.feeAddress || path != "/api/v3/feeaddress" {
					return apiError(types.ErrInternalError)
				}
				return http.StatusOK, types.FeeAddressResponse{
					Timestamp:  time.Now().Unix(),
					FeeAddress: ticket.votingAddr.String(),
					FeeAmount:  1e7,
					Expiration: time.Now().Add(time.Hour).Unix(),
					Request:    body,
				}
			})
			ticket = testVSPTicket(ctx, t, c, 1)
			// The wallet does not know this input, so signing the fee
			// tx fails after its outputs were written.
			purchaseTx := wire.NewMsgTx()
			purchaseTx.AddTxIn(wire.NewTxIn(&wire.OutPoint{Hash: chainhash.Hash{0xca}}, 1e8, nil))

			fp := c.feePayment(ctx, ticket, false)
			fp.stop()
			var attemptTx *wire.MsgTx
			if tc.attempt == "rejected" {
				// The job holds an earlier fee tx.
				old := wire.NewMsgTx()
				old.AddTxOut(wire.NewTxOut(1e7, []byte{0x52}))
				fp.feeTx, fp.feeHash = old, old.TxHash()
			}
			if tc.attempt != "" {
				fp.procMu.Lock() // the attempt is running
			}
			errc := make(chan error, 1)
			go func() { errc <- c.Process(ctx, ticket, purchaseTx) }()
			if tc.attempt != "" {
				waitBlockedOnMutex(t, "(*VSPClient).Process")
				var err error
				fp.mu.Lock()
				switch tc.attempt {
				case "pays":
					if fp.feeTx != nil {
						t.Error("Process gave the job the purchase's fee tx before it held the lock")
					}
					attemptTx = wire.NewMsgTx()
					attemptTx.AddTxOut(wire.NewTxOut(1e7, []byte{0x51}))
					fp.feeTx, fp.feeHash = attemptTx, attemptTx.TxHash()
					err = ticket.UpdateFeePaid(ctx, attemptTx.TxHash(), c.URL, c.PubKey)
				case "rejected":
					fp.feeTx, fp.feeHash = nil, chainhash.Hash{}
					err = ticket.UpdateFeeErrored(ctx, c.URL, c.PubKey)
				}
				fp.mu.Unlock()
				if err != nil {
					t.Fatal(err)
				}
				fp.procMu.Unlock()
			}
			err := <-errc

			var apiErr types.ErrorResponse
			var walletErr *errors.Error
			switch {
			case tc.feeAddress:
				if !errors.As(err, &walletErr) || walletErr.Op != "wallet.SignTransaction" {
					t.Fatalf("Process: want the signing error, got %v", err)
				}
				if len(purchaseTx.TxOut) == 0 {
					t.Fatal("test setup: the fee tx failed before its outputs were written")
				}
			case !errors.As(err, &apiErr) || apiErr.Code != types.ErrInternalError:
				t.Fatalf("Process: want the fee address error, got %v", err)
			}
			fp.mu.Lock()
			defer fp.mu.Unlock()
			if fp.feeTx != attemptTx {
				t.Fatalf("job's fee tx: got %p, want %p (purchase's %p)", fp.feeTx, attemptTx, purchaseTx)
			}
			want := udb.VSPFeeProcessErrored
			if tc.attempt == "pays" {
				want = udb.VSPFeeProcessPaid
			}
			info, err := ticket.VSPTicketInfo(ctx)
			if err != nil || udb.FeeStatus(info.FeeTxStatus) != want {
				t.Fatalf("fee status: %+v, %v; want %v", info, err, want)
			}
		})
	}
}

// signedFeeTx returns a fee tx that looks finished to the client: it has an
// output and a signed input.
func signedFeeTx(id byte) *wire.MsgTx {
	tx := wire.NewMsgTx()
	tx.AddTxIn(wire.NewTxIn(&wire.OutPoint{Hash: chainhash.Hash{id}}, 2e7, []byte{0x51}))
	tx.AddTxOut(wire.NewTxOut(1e7, []byte{0x51}))
	return tx
}

// insertUnmined records tx as an unmined wallet transaction, unpublished (as
// the wallet records a fee tx it built) or published.
func insertUnmined(ctx context.Context, t *testing.T, w *Wallet, tx *wire.MsgTx, unpublished bool) {
	t.Helper()
	err := walletdb.Update(ctx, w.db, func(dbtx walletdb.ReadWriteTx) error {
		rec, err := udb.NewTxRecordFromMsgTx(tx, time.Now())
		if err != nil {
			return err
		}
		rec.Unpublished = unpublished
		return w.txStore.InsertMemPoolTx(dbtx, rec)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// failVSPWrites makes every fee status write of c fail with errors.Invalid:
// the wallet keys the VSP's record by its URL, and bbolt refuses keys over
// 32 KiB.  Requests still reach the fake VSP, under a longer path.
func failVSPWrites(c *VSPClient) {
	c.URL += "/" + strings.Repeat("v", 1<<15)
}

// TestVSPExpiredFeeMarksErrored checks that after the VSP reports the fee as
// expired, the ticket is no longer recorded as paid with the abandoned tx.
func TestVSPExpiredFeeMarksErrored(t *testing.T) {
	for _, tc := range []struct {
		name       string
		published  bool // the job's fee tx is one the wallet published
		failWrites bool
	}{
		{name: "unpublished fee tx"},
		{name: "published fee tx", published: true},
		// The VSP's error is returned, not the failed write's.
		{name: "status write fails", failWrites: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			c := testVSPClient(ctx, t, func(string, []byte) (int, any) {
				return apiError(types.ErrFeeExpired)
			})
			ticket := testVSPTicket(ctx, t, c, 1)
			feeTx := signedFeeTx(0xcc)
			insertUnmined(ctx, t, c.wallet, feeTx, !tc.published)
			if err := ticket.UpdateFeePaid(ctx, feeTx.TxHash(), c.URL, c.PubKey); err != nil {
				t.Fatal(err)
			}
			if tc.failWrites {
				failVSPWrites(c)
			}
			fp := &vspFeePayment{
				client:  c,
				ctx:     ctx,
				ticket:  ticket,
				policy:  c.policy,
				params:  c.wallet.chainParams,
				fee:     1e7,
				feeAddr: ticket.votingAddr,
				feeHash: feeTx.TxHash(),
				feeTx:   feeTx,
			}
			c.jobs[*ticket.Hash()] = fp

			var apiErr types.ErrorResponse
			if err := fp.reconcilePayment(); !errors.As(err, &apiErr) || apiErr.Code != types.ErrFeeExpired {
				t.Fatalf("reconcilePayment: want ErrFeeExpired, got %v", err)
			}
			fp.stop()
			want := udb.VSPFeeProcessErrored
			if tc.failWrites {
				want = udb.VSPFeeProcessPaid
			}
			if got := feeStatus(ctx, t, ticket); got != want {
				t.Fatalf("fee status after an expired fee = %v, want %v", got, want)
			}
			fp.mu.Lock()
			feeHash := fp.feeHash
			fp.mu.Unlock()
			if feeHash != (chainhash.Hash{}) {
				t.Fatalf("payment still holds the abandoned fee hash %v", feeHash)
			}
			expired := feeTx.TxHash()
			_, _, err := c.wallet.TxBlock(ctx, &expired)
			if gone := errors.Is(err, errors.NotExist); gone == tc.published {
				t.Fatalf("expired fee tx gone from the wallet: %v, want %v (err %v)", gone, !tc.published, err)
			}
		})
	}
}

// walletFundedFeeTx returns an unsigned fee tx with one input, an output the
// wallet holds, for makeFeeTx to finish.
func walletFundedFeeTx(ctx context.Context, t *testing.T, w *Wallet) *wire.MsgTx {
	t.Helper()
	addr, err := w.NewInternalAddress(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	ver, script := addr.(Address).PaymentScript()
	fund := wire.NewMsgTx()
	fund.AddTxIn(wire.NewTxIn(&wire.OutPoint{Hash: chainhash.Hash{2}}, 1e8, nil))
	fund.AddTxOut(&wire.TxOut{Value: 1e8, Version: ver, PkScript: script})
	if err := w.AddTransaction(ctx, fund, nil); err != nil {
		t.Fatal(err)
	}
	feeTx := wire.NewMsgTx()
	feeTx.AddTxIn(wire.NewTxIn(&wire.OutPoint{Hash: fund.TxHash()}, 1e8, nil))
	return feeTx
}

// TestVSPRejectedNewFeeTxIsAbandoned checks that a fee tx reconcilePayment
// makes and the VSP then rejects in the same attempt is removed from the
// wallet, so its inputs can be spent again.
func TestVSPRejectedNewFeeTxIsAbandoned(t *testing.T) {
	ctx := context.Background()
	c := testVSPClient(ctx, t, func(string, []byte) (int, any) {
		return apiError(types.ErrInvalidFeeTx)
	})
	w := c.wallet
	ticket := testVSPTicket(ctx, t, c, 1)
	feeTx := walletFundedFeeTx(ctx, t, w)
	fp := &vspFeePayment{
		client:  c,
		ctx:     ctx,
		ticket:  ticket,
		policy:  c.policy,
		params:  w.chainParams,
		fee:     1e7,
		feeAddr: ticket.votingAddr,
		feeTx:   feeTx,
	}
	c.jobs[*ticket.Hash()] = fp

	var apiErr types.ErrorResponse
	if err := fp.reconcilePayment(); !errors.As(err, &apiErr) || apiErr.Code != types.ErrInvalidFeeTx {
		t.Fatalf("reconcilePayment: want ErrInvalidFeeTx, got %v", err)
	}
	fp.stop()
	if len(feeTx.TxOut) == 0 {
		t.Fatal("test setup: the fee tx was not made")
	}
	feeHash := feeTx.TxHash()
	if _, _, err := w.TxBlock(ctx, &feeHash); !errors.Is(err, errors.NotExist) {
		t.Fatalf("the rejected fee tx is still in the wallet (err %v)", err)
	}
}

// TestVSPRejectedFeeTxIsAbandoned checks that a fee tx the VSP rejects is
// removed from the wallet so its inputs can be spent again, and that a new fee
// is requested on the retry, even when the errored status cannot be written.
// When it can, the ticket is recorded as errored.
func TestVSPRejectedFeeTxIsAbandoned(t *testing.T) {
	for _, tc := range []struct {
		name       string
		code       types.ErrorCode
		failWrites bool
		published  bool
	}{
		{name: "invalid, status written", code: types.ErrInvalidFeeTx},
		{name: "invalid, status write fails", code: types.ErrInvalidFeeTx, failWrites: true},
		{name: "cannot broadcast", code: types.ErrCannotBroadcastFee},
		{name: "unknown outputs", code: types.ErrCannotBroadcastFeeUnknownOutputs},
		// The VSP names a tx the wallet published: it stays.
		{name: "published tx", code: types.ErrInvalidFeeTx, published: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			c := testVSPClient(ctx, t, func(string, []byte) (int, any) {
				return apiError(tc.code)
			})
			ticket := testVSPTicket(ctx, t, c, 1)
			feeTx := signedFeeTx(0xcd)
			insertUnmined(ctx, t, c.wallet, feeTx, !tc.published)
			feeHash := feeTx.TxHash()
			if err := ticket.UpdateFeeStarted(ctx, feeHash, c.URL, c.PubKey); err != nil {
				t.Fatal(err)
			}
			if tc.failWrites {
				failVSPWrites(c)
			}
			fp := &vspFeePayment{
				client:  c,
				ctx:     ctx,
				ticket:  ticket,
				policy:  c.policy,
				params:  c.wallet.chainParams,
				fee:     1e7,
				feeAddr: ticket.votingAddr,
				feeHash: feeHash,
				feeTx:   feeTx,
			}
			c.jobs[*ticket.Hash()] = fp

			var apiErr types.ErrorResponse
			err := fp.reconcilePayment()
			if !errors.As(err, &apiErr) || apiErr.Code != tc.code {
				t.Fatalf("reconcilePayment: want %v, got %v", tc.code, err)
			}
			if !timerSet(fp) {
				t.Fatal("no retry scheduled after the VSP rejected the fee tx")
			}
			fp.stop()
			_, _, err = c.wallet.TxBlock(ctx, &feeHash)
			if gone := errors.Is(err, errors.NotExist); gone == tc.published {
				t.Fatalf("fee tx gone from the wallet: %v, want %v (err %v)", gone, !tc.published, err)
			}
			fp.mu.Lock()
			fee := fp.fee
			fp.mu.Unlock()
			if fee != 0 {
				t.Fatalf("the retry would pay the rejected fee %v again", fee)
			}
			want := udb.VSPFeeProcessErrored
			if tc.failWrites {
				want = udb.VSPFeeProcessStarted
			}
			if got := feeStatus(ctx, t, ticket); got != want {
				t.Fatalf("fee status after the VSP rejected the fee tx = %v, want %v", got, want)
			}
		})
	}
}

// TestVSPReconcileRetriesFailedPaidWrite checks that when the VSP accepts the
// fee but the wallet cannot record it as paid, the payment is retried rather
// than left with nothing scheduled.
func TestVSPReconcileRetriesFailedPaidWrite(t *testing.T) {
	ctx := context.Background()
	c := testVSPClient(ctx, t, func(path string, body []byte) (int, any) {
		if !strings.HasSuffix(path, "/api/v3/payfee") {
			return apiError(types.ErrInternalError)
		}
		return http.StatusOK, types.PayFeeResponse{
			Timestamp: time.Now().Unix(),
			Request:   body,
		}
	})
	ticket := testVSPTicket(ctx, t, c, 1)
	failVSPWrites(c)
	feeTx := signedFeeTx(0xcf)
	fp := &vspFeePayment{
		client:  c,
		ctx:     ctx,
		ticket:  ticket,
		policy:  c.policy,
		params:  c.wallet.chainParams,
		fee:     1e7,
		feeAddr: ticket.votingAddr,
		feeHash: feeTx.TxHash(),
		feeTx:   feeTx,
	}
	c.jobs[*ticket.Hash()] = fp

	if err := fp.reconcilePayment(); !errors.Is(err, errors.Invalid) {
		t.Fatalf("reconcilePayment: want the failed write, got %v", err)
	}
	if !timerSet(fp) {
		t.Fatal("a failed write after the VSP took the fee left no retry")
	}
}

// TestVSPFailedPaidWriteAbandonsFeeTx checks that a fee tx made and stored in
// the wallet is removed again when the ticket cannot be recorded as paid, so
// it does not hold its inputs while no payment refers to it.
func TestVSPFailedPaidWriteAbandonsFeeTx(t *testing.T) {
	ctx := context.Background()
	c := testVSPClient(ctx, t, func(string, []byte) (int, any) {
		return apiError(types.ErrInternalError)
	})
	w := c.wallet
	ticket := testVSPTicket(ctx, t, c, 1)
	feeTx := walletFundedFeeTx(ctx, t, w)
	failVSPWrites(c)
	fp := &vspFeePayment{
		client:  c,
		ctx:     ctx,
		ticket:  ticket,
		policy:  c.policy,
		params:  w.chainParams,
		fee:     1e7,
		feeAddr: ticket.votingAddr,
	}

	c.jobs[*ticket.Hash()] = fp
	fp.feeTx = feeTx
	if err := fp.reconcilePayment(); !errors.Is(err, errors.Invalid) {
		t.Fatalf("reconcilePayment: want the failed write, got %v", err)
	}
	fp.stop()
	if len(feeTx.TxOut) == 0 {
		t.Fatal("test setup: the fee tx was not made")
	}
	feeHash := feeTx.TxHash()
	if _, _, err := w.TxBlock(ctx, &feeHash); !errors.Is(err, errors.NotExist) {
		t.Fatalf("fee tx with no recorded payment is still in the wallet (err %v)", err)
	}
}

// TestVSPProcessManagedTicketsSkipsBadTicket checks that an error for one
// ticket does not stop the remaining tickets from being resumed.
func TestVSPProcessManagedTicketsSkipsBadTicket(t *testing.T) {
	validHash := chainhash.Hash{0xfe}.String()
	for _, tc := range []struct {
		name       string
		status     string
		feeHash    string
		failWrites bool
		// locked locks the wallet once the bad ticket's status is
		// answered, so the next ticket's request fails.
		locked bool
	}{
		{name: "bad confirmed fee hash", status: "confirmed", feeHash: "not a hash"},
		{name: "bad paid fee hash", status: "broadcast", feeHash: "not a hash"},
		{name: "confirmed write fails", status: "confirmed", feeHash: validHash, failWrites: true},
		{name: "paid write fails", status: "broadcast", feeHash: validHash, failWrites: true},
		{name: "wallet locks after a bad ticket", status: "confirmed", feeHash: "not a hash", locked: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			var badHash string // set once the ticket exists
			var c *VSPClient
			c = testVSPClient(ctx, t, func(path string, body []byte) (int, any) {
				if !strings.HasSuffix(path, "/api/v3/ticketstatus") {
					return apiError(types.ErrInternalError)
				}
				var req types.TicketStatusRequest
				if err := json.Unmarshal(body, &req); err != nil {
					return apiError(types.ErrBadRequest)
				}
				if req.TicketHash == badHash {
					if tc.locked {
						c.wallet.Lock()
					}
					return http.StatusOK, types.TicketStatusResponse{
						FeeTxStatus: tc.status,
						FeeTxHash:   tc.feeHash,
						Request:     body,
					}
				}
				return http.StatusOK, types.TicketStatusResponse{FeeTxStatus: "none", Request: body}
			})
			bad := testVSPTicket(ctx, t, c, 1)
			unpaid := testVSPTicket(ctx, t, c, 2)
			badHash = bad.Hash().String()
			if tc.failWrites {
				failVSPWrites(c)
			}

			err := c.ProcessManagedTickets(ctx, []*VSPTicket{bad, unpaid})
			if err == nil || !strings.Contains(err.Error(), badHash) {
				t.Fatalf("ProcessManagedTickets: want the bad ticket's error, got %v", err)
			}
			if tc.locked {
				if !errors.Is(err, errors.Locked) {
					t.Fatalf("ProcessManagedTickets: want the Locked error too, got %v", err)
				}
				return
			}
			c.mu.Lock()
			_, resumed := c.jobs[*unpaid.Hash()]
			_, badJob := c.jobs[*bad.Hash()]
			c.mu.Unlock()
			if badJob {
				t.Fatal("a job was started for the ticket whose status could not be recorded")
			}
			if !resumed {
				t.Fatal("an error for one ticket stopped the next ticket")
			}
		})
	}
}

// waitBlockedOnMutex returns once a goroutine running fn waits to lock a
// mutex.
func waitBlockedOnMutex(t *testing.T, fn string) {
	t.Helper()
	buf := make([]byte, 1<<20)
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); {
		n := runtime.Stack(buf, true)
		for _, g := range strings.Split(string(buf[:n]), "\n\n") {
			if strings.Contains(g, "[sync.Mutex.Lock") && strings.Contains(g, fn) {
				return
			}
		}
		runtime.Gosched()
	}
	t.Fatalf("no goroutine in %s waited for a mutex", fn)
}

// TestVSPFeeWorkHoldsProcessingLock checks that Process and a scheduled
// attempt make every VSP request for a ticket while holding its processing
// lock, so the two cannot build or submit that ticket's fee tx at once.
func TestVSPFeeWorkHoldsProcessingLock(t *testing.T) {
	ctx := context.Background()
	var fp *vspFeePayment
	var ticket *VSPTicket
	var requests, unlocked, payFees atomic.Int32
	var quote atomic.Bool // answer feeaddress with a fee
	c := testVSPClient(ctx, t, func(path string, body []byte) (int, any) {
		requests.Add(1)
		if fp.procMu.TryLock() {
			fp.procMu.Unlock()
			unlocked.Add(1)
		}
		switch {
		case path == "/api/v3/payfee":
			payFees.Add(1)
		case path == "/api/v3/feeaddress" && quote.Load():
			return http.StatusOK, types.FeeAddressResponse{
				Timestamp:  time.Now().Unix(),
				FeeAddress: ticket.votingAddr.String(),
				FeeAmount:  1e7,
				Expiration: time.Now().Add(time.Hour).Unix(),
				Request:    body,
			}
		}
		return apiError(types.ErrInternalError)
	})
	ticket = testVSPTicket(ctx, t, c, 1)
	fp = &vspFeePayment{
		client: c,
		ctx:    ctx,
		ticket: ticket,
		policy: c.policy,
		params: c.wallet.chainParams,
	}
	c.mu.Lock()
	c.jobs[*ticket.Hash()] = fp
	c.mu.Unlock()

	check := func(name string, work func()) {
		t.Helper()
		before := requests.Load()
		work()
		if requests.Load() == before {
			t.Fatalf("%s made no VSP request", name)
		}
		if n := unlocked.Load(); n != 0 {
			t.Fatalf("%s made %d VSP request(s) without the processing lock", name, n)
		}
	}
	check("Process", func() { _ = c.Process(ctx, ticket, nil) })
	check("scheduled attempt", fp.task("reconcile payment", fp.reconcilePayment))
	fp.stop() // the attempt rescheduled itself; its timer would hold the lock

	// With a finished fee tx in the job, Process submits it.
	finished := wire.NewMsgTx()
	finished.AddTxOut(wire.NewTxOut(1e7, []byte{0x51}))
	fp.mu.Lock()
	fp.feeTx = finished
	fp.mu.Unlock()
	quote.Store(true)
	check("Process submitting a fee", func() { _ = c.Process(ctx, ticket, nil) })
	if payFees.Load() == 0 {
		t.Fatal("test setup: Process did not submit the fee")
	}
	quote.Store(false)
	if err := ticket.UpdateFeePaid(ctx, chainhash.Hash{0xfe}, c.URL, c.PubKey); err != nil {
		t.Fatal(err)
	}
	check("Process of a paid ticket", func() { _ = c.Process(ctx, ticket, nil) })
}

func managed(c *VSPClient, ticket *VSPTicket) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.jobs[*ticket.Hash()]
	return ok
}

// prunedTicket returns kept's ticket data under a hash the wallet does not have.
func prunedTicket(c *VSPClient, kept *VSPTicket) *VSPTicket {
	h := chainhash.Hash{0xdd}
	return &VSPTicket{
		hash:           &h,
		rawTx:          kept.rawTx,
		parentTx:       kept.parentTx,
		commitmentAddr: kept.commitmentAddr,
		votingAddr:     kept.votingAddr,
		wallet:         c.wallet,
	}
}

// TestVSPFeeTooHighStopsRetries checks that a fee above the configured maximum
// is not retried every few minutes but marked errored for the next Process
// call.
func TestVSPFeeTooHighStopsRetries(t *testing.T) {
	// The job may start with no fee hash, or keep the hash of an earlier
	// fee tx, as after an expired fee.
	for name, start := range map[string]chainhash.Hash{
		"no fee hash":      {},
		"earlier fee hash": {0xab},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			var ticket *VSPTicket
			c := testVSPClient(ctx, t, func(path string, body []byte) (int, any) {
				if path != "/api/v3/feeaddress" {
					return apiError(types.ErrInternalError)
				}
				return http.StatusOK, types.FeeAddressResponse{
					Timestamp:  time.Now().Unix(),
					FeeAddress: ticket.votingAddr.String(),
					FeeAmount:  2e8, // above the client's 1e8 MaxFee
					Expiration: time.Now().Add(time.Hour).Unix(),
					Request:    body,
				}
			})
			ticket = testVSPTicket(ctx, t, c, 1)
			if err := ticket.UpdateFeeStarted(ctx, start, c.URL, c.PubKey); err != nil {
				t.Fatal(err)
			}
			fp := &vspFeePayment{
				client:  c,
				ctx:     ctx,
				ticket:  ticket,
				feeHash: start,
				policy:  c.policy,
				params:  c.wallet.chainParams,
			}
			c.jobs[*ticket.Hash()] = fp

			if err := fp.reconcilePayment(); !errors.Is(err, errFeeTooHigh) {
				t.Fatalf("reconcilePayment: want errFeeTooHigh, got %v", err)
			}
			if timerSet(fp) {
				t.Fatal("a fee above the maximum was scheduled for another attempt")
			}
			if managed(c, ticket) {
				t.Fatal("a fee above the maximum left the ticket managed by the client")
			}
			if got := feeStatus(ctx, t, ticket); got != udb.VSPFeeProcessErrored {
				t.Fatalf("fee status = %v, want errored", got)
			}
		})
	}
}

// TestVSPFeeTooHighKeepsConcurrentPayment checks that a quote above the maximum
// does not mark errored or remove a payment that a concurrent Process call
// made while the attempt waited for the quote.
func TestVSPFeeTooHighKeepsConcurrentPayment(t *testing.T) {
	ctx := context.Background()
	var ticket *VSPTicket
	var fp *vspFeePayment
	paid := chainhash.Hash{0xfe}
	c := testVSPClient(ctx, t, func(path string, body []byte) (int, any) {
		if path != "/api/v3/feeaddress" {
			return apiError(types.ErrInternalError)
		}
		// A concurrent Process call makes and records a fee tx.
		fp.mu.Lock()
		fp.feeHash = paid
		fp.mu.Unlock()
		if err := ticket.UpdateFeePaid(ctx, paid, fp.client.URL, fp.client.PubKey); err != nil {
			t.Error(err)
		}
		return http.StatusOK, types.FeeAddressResponse{
			Timestamp:  time.Now().Unix(),
			FeeAddress: ticket.votingAddr.String(),
			FeeAmount:  2e8, // above the client's 1e8 MaxFee
			Expiration: time.Now().Add(time.Hour).Unix(),
			Request:    body,
		}
	})
	ticket = testVSPTicket(ctx, t, c, 1)
	fp = &vspFeePayment{
		client: c,
		ctx:    ctx,
		ticket: ticket,
		policy: c.policy,
		params: c.wallet.chainParams,
	}
	c.jobs[*ticket.Hash()] = fp

	if err := fp.reconcilePayment(); !errors.Is(err, errFeeTooHigh) {
		t.Fatalf("reconcilePayment: want errFeeTooHigh, got %v", err)
	}
	if !managed(c, ticket) {
		t.Fatal("the concurrent payment's job was removed")
	}
	if !timerSet(fp) {
		t.Fatal("the concurrent payment's job has nothing scheduled")
	}
	fp.stop()
	if got := feeStatus(ctx, t, ticket); got != udb.VSPFeeProcessPaid {
		t.Fatalf("fee status = %v, want paid", got)
	}
}

// TestVSPPrunedTicketIsRemoved checks that a ticket the wallet no longer has
// is dropped from the client instead of being retried, with its VSP record
// kept, and that an unmined ticket the wallet still has keeps its job.
func TestVSPPrunedTicketIsRemoved(t *testing.T) {
	ctx := context.Background()
	c := testVSPClient(ctx, t, func(string, []byte) (int, any) {
		return apiError(types.ErrInternalError)
	})
	kept := testVSPTicket(ctx, t, c, 1) // unmined, in the wallet
	pruned := prunedTicket(c, kept)
	jobs := make(map[*VSPTicket]*vspFeePayment)
	for _, ticket := range []*VSPTicket{kept, pruned} {
		if err := ticket.UpdateFeeStarted(ctx, chainhash.Hash{}, c.URL, c.PubKey); err != nil {
			t.Fatal(err)
		}
		fp := &vspFeePayment{
			client: c,
			ctx:    ctx,
			ticket: ticket,
			policy: c.policy,
			params: c.wallet.chainParams,
		}
		c.jobs[*ticket.Hash()] = fp
		jobs[ticket] = fp
	}

	if err := jobs[pruned].reconcilePayment(); !errors.Is(err, errStopped) {
		t.Fatalf("pruned ticket: reconcilePayment: want errStopped, got %v", err)
	}
	if managed(c, pruned) {
		t.Fatal("the pruned ticket is still managed by the client")
	}
	if timerSet(jobs[pruned]) {
		t.Fatal("the pruned ticket was scheduled for another attempt")
	}
	if _, err := pruned.VSPTicketInfo(ctx); err != nil {
		t.Fatalf("the pruned ticket's VSP record is gone: %v", err)
	}

	// The fake VSP fails the quote, so the unmined ticket is retried.
	if err := jobs[kept].reconcilePayment(); err == nil || errors.Is(err, errStopped) {
		t.Fatalf("unmined ticket: reconcilePayment: want the VSP error, got %v", err)
	}
	if !managed(c, kept) {
		t.Fatal("the unmined ticket was removed from the client")
	}
	if !timerSet(jobs[kept]) {
		t.Fatal("the unmined ticket was not scheduled for another attempt")
	}
	if got := feeStatus(ctx, t, kept); got != udb.VSPFeeProcessStarted {
		t.Fatalf("unmined ticket fee status = %v, want started", got)
	}

	// A database that cannot be read says nothing about the ticket.
	jobs[kept].stop()
	if err := c.wallet.db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := jobs[kept].reconcilePayment(); errors.Is(err, errStopped) {
		t.Fatal("a database error was taken for a pruned ticket")
	}
	jobs[kept].stop()
}

// TestVSPPrunedTicketStopsBeforeVSPRequests checks that a pruned ticket's
// payment stops before any VSP request on each path into it: Process of an
// unpaid ticket (fee address), Process of a paid one (status), and a scheduled
// attempt whose job already holds a fee tx (fee submission).
func TestVSPPrunedTicketStopsBeforeVSPRequests(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func(ctx context.Context, t *testing.T, c *VSPClient, ticket *VSPTicket) error
	}{{
		name: "Process, fee not paid",
		run: func(ctx context.Context, t *testing.T, c *VSPClient, ticket *VSPTicket) error {
			return c.Process(ctx, ticket, nil)
		},
	}, {
		name: "Process, fee paid",
		run: func(ctx context.Context, t *testing.T, c *VSPClient, ticket *VSPTicket) error {
			if err := ticket.UpdateFeePaid(ctx, chainhash.Hash{0xee}, c.URL, c.PubKey); err != nil {
				t.Fatal(err)
			}
			return c.Process(ctx, ticket, nil)
		},
	}, {
		name: "scheduled attempt with a fee tx",
		run: func(ctx context.Context, t *testing.T, c *VSPClient, ticket *VSPTicket) error {
			feeTx := wire.NewMsgTx()
			feeTx.AddTxIn(wire.NewTxIn(&wire.OutPoint{Hash: chainhash.Hash{0xef}}, 1e7, nil))
			feeTx.AddTxOut(&wire.TxOut{Value: 1e7})
			fp := &vspFeePayment{client: c, ctx: ctx, ticket: ticket, feeTx: feeTx}
			c.jobs[*ticket.Hash()] = fp
			err := fp.reconcilePayment()
			if timerSet(fp) {
				t.Fatal("the removed job was scheduled for another attempt")
			}
			return err
		},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			var requests atomic.Int32
			c := testVSPClient(ctx, t, func(string, []byte) (int, any) {
				requests.Add(1)
				return apiError(types.ErrInternalError)
			})
			pruned := prunedTicket(c, testVSPTicket(ctx, t, c, 1))

			if err := tc.run(ctx, t, c, pruned); !errors.Is(err, errStopped) {
				t.Fatalf("want errStopped, got %v", err)
			}
			if n := requests.Load(); n != 0 {
				t.Fatalf("%d requests sent to the VSP for a pruned ticket", n)
			}
			if managed(c, pruned) {
				t.Fatal("the pruned ticket is still managed by the client")
			}
		})
	}
}

// TestVSPTicketPrunedDuringProcessStops checks that a ticket the wallet drops
// while Process waits for the fee address is not paid for: submitPayment
// stops before the payfee request.
func TestVSPTicketPrunedDuringProcessStops(t *testing.T) {
	ctx := context.Background()
	var c *VSPClient
	var ticket *VSPTicket
	var payfee atomic.Int32
	c = testVSPClient(ctx, t, func(path string, body []byte) (int, any) {
		switch path {
		case "/api/v3/feeaddress":
			if err := c.wallet.AbandonTransaction(ctx, ticket.Hash()); err != nil {
				t.Error(err)
			}
			return http.StatusOK, types.FeeAddressResponse{
				Timestamp:  time.Now().Unix(),
				FeeAddress: ticket.votingAddr.String(),
				FeeAmount:  1e7,
				Expiration: time.Now().Add(time.Hour).Unix(),
				Request:    body,
			}
		case "/api/v3/payfee":
			payfee.Add(1)
		}
		return apiError(types.ErrInternalError)
	})
	ticket = testVSPTicket(ctx, t, c, 1)

	// Fund the fee tx with a wallet output.
	addr, err := c.wallet.NewInternalAddress(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	ver, script := addr.(Address).PaymentScript()
	fund := wire.NewMsgTx()
	fund.AddTxIn(wire.NewTxIn(&wire.OutPoint{Hash: chainhash.Hash{2}}, 1e8, nil))
	fund.AddTxOut(&wire.TxOut{Value: 1e8, Version: ver, PkScript: script})
	if err := c.wallet.AddTransaction(ctx, fund, nil); err != nil {
		t.Fatal(err)
	}
	feeTx := wire.NewMsgTx()
	feeTx.AddTxIn(wire.NewTxIn(&wire.OutPoint{Hash: fund.TxHash()}, 1e8, nil))

	if err := c.Process(ctx, ticket, feeTx); !errors.Is(err, errStopped) {
		t.Fatalf("Process: want errStopped, got %v", err)
	}
	if n := payfee.Load(); n != 0 {
		t.Fatalf("%d payfee requests for a ticket pruned during Process", n)
	}
	if managed(c, ticket) {
		t.Fatal("the pruned ticket is still managed by the client")
	}
}
