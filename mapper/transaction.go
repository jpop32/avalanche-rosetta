package mapper

import (
	"log"
	"math/big"
	"reflect"
	"strings"

	ethtypes "github.com/ava-labs/coreth/core/types"
	"github.com/ava-labs/coreth/plugin/evm"
	"github.com/coinbase/rosetta-sdk-go/types"

	"github.com/figment-networks/avalanche-rosetta/client"
)

var (
	x2crate = big.NewInt(1000000000)
)

func Transaction(
	header *ethtypes.Header,
	tx *ethtypes.Transaction,
	msg *ethtypes.Message,
	receipt *ethtypes.Receipt,
	trace *client.Call,
) (*types.Transaction, error) {
	ops := []*types.Operation{}
	sender := msg.From()
	feeReceiver := &header.Coinbase

	txFee := new(big.Int).SetUint64(receipt.GasUsed)
	txFee = txFee.Mul(txFee, msg.GasPrice())

	feeOps := []*types.Operation{
		{
			OperationIdentifier: &types.OperationIdentifier{
				Index: 0,
			},
			Type:    OpFee,
			Status:  types.String(StatusSuccess),
			Account: Account(&sender),
			Amount:  AvaxAmount(new(big.Int).Neg(txFee)),
		},
		{
			OperationIdentifier: &types.OperationIdentifier{
				Index: 1,
			},
			RelatedOperations: []*types.OperationIdentifier{
				{
					Index: 0,
				},
			},
			Type:    OpFee,
			Status:  types.String(StatusSuccess),
			Account: Account(feeReceiver),
			Amount:  AvaxAmount(txFee),
		},
	}

	ops = append(ops, feeOps...)

	traces := client.FlattenTraces(trace, []*client.FlatCall{})
	traceOps := traceOps(traces, len(feeOps))
	ops = append(ops, traceOps...)

	return &types.Transaction{
		TransactionIdentifier: &types.TransactionIdentifier{
			Hash: tx.Hash().String(),
		},
		Operations: ops,
		Metadata: map[string]interface{}{
			"gas":       tx.Gas(),
			"gas_price": tx.GasPrice().String(),
			"receipt":   receipt,
			"trace":     trace,
		},
	}, nil
}

func CrossChainTransactions(block *ethtypes.Block) ([]*types.Transaction, error) {
	transactions := []*types.Transaction{}

	extra := block.ExtraData()
	if len(extra) == 0 {
		return transactions, nil
	}

	tx := &evm.Tx{}
	if _, err := codecManager.Unmarshal(extra, tx); err != nil {
		return nil, err
	}

	var idx int64
	var txID string

	ops := []*types.Operation{}

	switch t := tx.UnsignedTx.(type) {
	case *evm.UnsignedImportTx:
		for _, in := range t.ImportedInputs {
			txID = in.TxID.String()
			break
		}

		for _, out := range t.Outs {
			op := &types.Operation{
				OperationIdentifier: &types.OperationIdentifier{
					Index: idx,
				},
				Type:   OpImport,
				Status: types.String(StatusSuccess),
				Account: &types.AccountIdentifier{
					Address: out.Address.Hex(),
				},
				Amount: &types.Amount{
					Value:    new(big.Int).Mul(new(big.Int).SetUint64(out.Amount), x2crate).String(),
					Currency: AvaxCurrency,
				},
				Metadata: map[string]interface{}{
					"tx_id":         txID,
					"blockchain_id": t.BlockchainID.String(),
					"network_id":    t.NetworkID,
					"source_chain":  t.SourceChain.String(),
					"meta":          t.Metadata,
					"asset_id":      out.AssetID.String(),
				},
			}
			ops = append(ops, op)
			idx++
		}
	case *evm.UnsignedExportTx:
		for _, in := range t.Ins {
			op := &types.Operation{
				OperationIdentifier: &types.OperationIdentifier{
					Index: idx,
				},
				Type:   OpExport,
				Status: types.String(StatusSuccess),
				Account: &types.AccountIdentifier{
					Address: in.Address.Hex(),
				},
				Amount: &types.Amount{
					Value:    new(big.Int).Mul(new(big.Int).SetUint64(in.Amount), new(big.Int).Neg(x2crate)).String(),
					Currency: AvaxCurrency,
				},
				Metadata: map[string]interface{}{
					"blockchain_id":     t.BlockchainID.String(),
					"network_id":        t.NetworkID,
					"destination_chain": t.DestinationChain.String(),
					"meta":              t.Metadata,
					"asset_id":          in.AssetID.String(),
				},
			}
			ops = append(ops, op)
			idx++
		}
	default:
		panic("Unsupported transaction:" + reflect.TypeOf(t).String())
	}

	transactions = append(transactions, &types.Transaction{
		TransactionIdentifier: &types.TransactionIdentifier{
			Hash: block.Hash().String(),
		},
		Operations: ops,
	})

	return transactions, nil
}

// MempoolTransactionsIDs returns a list of transction IDs in the mempool
// TODO: is not really needed but included for API completion
func MempoolTransactionsIDs(accountMap client.TxAccountMap) []*types.TransactionIdentifier {
	result := []*types.TransactionIdentifier{}

	for _, txNonceMap := range accountMap {
		for _, tx := range txNonceMap {
			// todo: this should be a parsed out struct from the eth client
			chunks := strings.Split(tx, ":")

			result = append(result, &types.TransactionIdentifier{
				Hash: chunks[0],
			})
		}
	}

	return result
}

func traceOps(calls []*client.FlatCall, startIndex int) []*types.Operation {
	var ops []*types.Operation
	if len(calls) == 0 {
		return ops
	}

	destroyedAccounts := map[string]*big.Int{}
	for _, trace := range calls {
		// Handle partial transaction success
		metadata := map[string]interface{}{}
		opStatus := StatusSuccess
		if trace.Revert {
			opStatus = StatusFailure
			metadata["error"] = trace.ErrorMessage
		}

		var zeroValue bool
		if trace.Value.Sign() == 0 {
			zeroValue = true
		}

		// Skip all 0 value CallType operations (TODO: make optional to include)
		//
		// We can't continue here because we may need to adjust our destroyed
		// accounts map if a CallTYpe operation resurrects an account.
		shouldAdd := true
		if zeroValue && CallType(trace.Type) {
			shouldAdd = false
		}

		// Checksum addresses
		from := trace.From.String()
		to := trace.To.String()

		if shouldAdd {
			fromOp := &types.Operation{
				OperationIdentifier: &types.OperationIdentifier{
					Index: int64(len(ops) + startIndex),
				},
				Type:   trace.Type,
				Status: types.String(opStatus),
				Account: &types.AccountIdentifier{
					Address: from,
				},
				Amount: &types.Amount{
					Value:    new(big.Int).Neg(trace.Value).String(),
					Currency: AvaxCurrency,
				},
				Metadata: metadata,
			}
			if zeroValue {
				fromOp.Amount = nil
			} else {
				_, destroyed := destroyedAccounts[from]
				if destroyed && opStatus == StatusSuccess {
					destroyedAccounts[from] = new(big.Int).Sub(destroyedAccounts[from], trace.Value)
				}
			}

			ops = append(ops, fromOp)
		}

		// Add to destroyed accounts if SELFDESTRUCT
		// and overwrite existing balance.
		if trace.Type == OpSelfDestruct {
			destroyedAccounts[from] = new(big.Int)

			// If destination of of SELFDESTRUCT is self,
			// we should skip. In the EVM, the balance is reset
			// after the balance is increased on the destination
			// so this is a no-op.
			if from == to {
				continue
			}
		}

		// Skip empty to addresses (this may not
		// actually occur but leaving it as a
		// sanity check)
		if len(trace.To.String()) == 0 {
			continue
		}

		// If the account is resurrected, we remove it from
		// the destroyed accounts map.
		if CreateType(trace.Type) {
			delete(destroyedAccounts, to)
		}

		if shouldAdd {
			lastOpIndex := ops[len(ops)-1].OperationIdentifier.Index
			toOp := &types.Operation{
				OperationIdentifier: &types.OperationIdentifier{
					Index: lastOpIndex + 1,
				},
				RelatedOperations: []*types.OperationIdentifier{
					{
						Index: lastOpIndex,
					},
				},
				Type:   trace.Type,
				Status: types.String(opStatus),
				Account: &types.AccountIdentifier{
					Address: to,
				},
				Amount: &types.Amount{
					Value:    trace.Value.String(),
					Currency: AvaxCurrency,
				},
				Metadata: metadata,
			}
			if zeroValue {
				toOp.Amount = nil
			} else {
				_, destroyed := destroyedAccounts[to]
				if destroyed && opStatus == StatusSuccess {
					destroyedAccounts[to] = new(big.Int).Add(destroyedAccounts[to], trace.Value)
				}
			}

			ops = append(ops, toOp)
		}
	}

	// Zero-out all destroyed accounts that are removed
	// during transaction finalization.
	for acct, val := range destroyedAccounts {
		if val.Sign() == 0 {
			continue
		}

		if val.Sign() < 0 {
			log.Fatalf("negative balance for suicided account %s: %s\n", acct, val.String())
		}

		ops = append(ops, &types.Operation{
			OperationIdentifier: &types.OperationIdentifier{
				Index: ops[len(ops)-1].OperationIdentifier.Index + 1,
			},
			Type:   OpDestruct,
			Status: types.String(StatusSuccess),
			Account: &types.AccountIdentifier{
				Address: acct,
			},
			Amount: &types.Amount{
				Value:    new(big.Int).Neg(val).String(),
				Currency: AvaxCurrency,
			},
		})
	}

	return ops
}
