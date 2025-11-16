package jetton

import (
	"context"
	"fmt"
	"math/big"

	"github.com/xssnick/tonutils-go/address"
	"github.com/xssnick/tonutils-go/tlb"
	"github.com/xssnick/tonutils-go/ton"
	"github.com/xssnick/tonutils-go/tvm/cell"
)

const (
	// OpTransfer is the opcode for jetton transfer (TEP-74)
	OpTransfer uint32 = 0x0f8a7ea5
	// OpInternalTransfer is the opcode for jetton internal transfer
	OpInternalTransfer uint32 = 0x178d4519
	// OpTransferNotification is the opcode for jetton transfer notification
	OpTransferNotification uint32 = 0x7362d09c
)

// JettonTransfer represents a parsed jetton transfer
type JettonTransfer struct {
	Opcode       uint32
	Amount       *big.Int
	Destination  *address.Address
	ResponseDest *address.Address
	ForwardAmount *big.Int
}

// JettonMetadata contains jetton token information
type JettonMetadata struct {
	TotalSupply *big.Int
	Mintable    bool
	AdminAddr   *address.Address
	ContentCell *cell.Cell
	WalletCode  *cell.Cell
	Symbol      string
	Name        string
	Decimals    int
}

// ParseJettonTransfer attempts to parse a jetton transfer from message payload
func ParseJettonTransfer(msg *tlb.InternalMessage) (*JettonTransfer, error) {
	payloadCell := msg.Payload()
	if payloadCell == nil {
		return nil, nil
	}

	payload := payloadCell.BeginParse()
	if payload.BitsLeft() < 32 {
		return nil, nil
	}

	// Read opcode
	opcode64, err := payload.LoadUInt(32)
	if err != nil {
		return nil, nil
	}

	opcode := uint32(opcode64)

	// Check if it's a jetton transfer opcode
	if opcode != OpTransfer && opcode != OpInternalTransfer && opcode != OpTransferNotification {
		return nil, nil
	}

	transfer := &JettonTransfer{
		Opcode: opcode,
	}

	switch uint32(opcode) {
	case OpTransfer:
		// 0x0f8a7ea5 query_id:uint64 amount:(VarUInteger 16) destination:MsgAddress
		// response_destination:MsgAddress custom_payload:(Maybe ^Cell)
		// forward_ton_amount:(VarUInteger 16) forward_payload:(Either Cell ^Cell)

		// Skip query_id
		if payload.BitsLeft() < 64 {
			return nil, nil
		}
		_, err := payload.LoadUInt(64)
		if err != nil {
			return nil, nil
		}

		// Load amount (VarUInteger 16)
		amountVal, err := payload.LoadVarUInt(16)
		if err != nil {
			return nil, nil
		}
		transfer.Amount = amountVal

		// Load destination
		dest, err := payload.LoadAddr()
		if err != nil {
			return nil, nil
		}
		transfer.Destination = dest

		// Load response_destination
		respDest, err := payload.LoadAddr()
		if err != nil {
			return nil, nil
		}
		transfer.ResponseDest = respDest

		// Skip custom_payload (Maybe ^Cell)
		hasCustomPayload, err := payload.LoadBoolBit()
		if err != nil {
			return nil, nil
		}
		if hasCustomPayload {
			_, err = payload.LoadRef()
			if err != nil {
				return nil, nil
			}
		}

		// Load forward_ton_amount (VarUInteger 16)
		forwardAmountVal, err := payload.LoadVarUInt(16)
		if err != nil {
			return nil, nil
		}
		transfer.ForwardAmount = forwardAmountVal

	case OpInternalTransfer:
		// 0x178d4519 query_id:uint64 amount:(VarUInteger 16) from:MsgAddress
		// response_address:MsgAddress forward_ton_amount:(VarUInteger 16)
		// forward_payload:(Either Cell ^Cell)

		// Skip query_id
		if payload.BitsLeft() < 64 {
			return nil, nil
		}
		_, err := payload.LoadUInt(64)
		if err != nil {
			return nil, nil
		}

		// Load amount (VarUInteger 16)
		amountVal, err := payload.LoadVarUInt(16)
		if err != nil {
			return nil, nil
		}
		transfer.Amount = amountVal

		// Load from address
		from, err := payload.LoadAddr()
		if err != nil {
			return nil, nil
		}
		transfer.Destination = from

		// Load response_address
		respAddr, err := payload.LoadAddr()
		if err != nil {
			return nil, nil
		}
		transfer.ResponseDest = respAddr

		// Load forward_ton_amount
		forwardAmountVal, err := payload.LoadVarUInt(16)
		if err != nil {
			return nil, nil
		}
		transfer.ForwardAmount = forwardAmountVal

	case OpTransferNotification:
		// 0x7362d09c query_id:uint64 amount:(VarUInteger 16)
		// sender:MsgAddress forward_payload:(Either Cell ^Cell)

		// Skip query_id
		if payload.BitsLeft() < 64 {
			return nil, nil
		}
		_, err := payload.LoadUInt(64)
		if err != nil {
			return nil, nil
		}

		// Load amount
		amountVal, err := payload.LoadVarUInt(16)
		if err != nil {
			return nil, nil
		}
		transfer.Amount = amountVal

		// Load sender
		sender, err := payload.LoadAddr()
		if err != nil {
			return nil, nil
		}
		transfer.Destination = sender
	}

	return transfer, nil
}

// GetJettonMetadata fetches metadata from a jetton master contract
func GetJettonMetadata(ctx context.Context, api ton.APIClientWrapped, jettonMaster *address.Address) (*JettonMetadata, error) {
	// Get master block
	master, err := api.CurrentMasterchainInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get master block: %w", err)
	}

	// Call get_jetton_data() method
	res, err := api.RunGetMethod(ctx, master, jettonMaster, "get_jetton_data")
	if err != nil {
		return nil, fmt.Errorf("failed to call get_jetton_data: %w", err)
	}

	if len(res.AsTuple()) < 5 {
		return nil, fmt.Errorf("unexpected response from get_jetton_data")
	}

	metadata := &JettonMetadata{}

	// Parse response: total_supply:int mintable:int admin_address:MsgAddress content:^Cell jetton_wallet_code:^Cell
	totalSupply, ok := res.AsTuple()[0].(*big.Int)
	if ok {
		metadata.TotalSupply = totalSupply
	}

	mintable, ok := res.AsTuple()[1].(*big.Int)
	if ok {
		metadata.Mintable = mintable.Cmp(big.NewInt(0)) != 0
	}

	adminSlice, ok := res.AsTuple()[2].(*cell.Slice)
	if ok {
		adminAddr, err := adminSlice.LoadAddr()
		if err == nil {
			metadata.AdminAddr = adminAddr
		}
	}

	contentCell, ok := res.AsTuple()[3].(*cell.Cell)
	if ok {
		metadata.ContentCell = contentCell
		// Parse metadata from content cell
		parseContentCell(contentCell, metadata)
	}

	walletCode, ok := res.AsTuple()[4].(*cell.Cell)
	if ok {
		metadata.WalletCode = walletCode
	}

	// Set defaults if not found
	if metadata.Symbol == "" {
		metadata.Symbol = "UNKNOWN"
	}
	if metadata.Name == "" {
		metadata.Name = "Unknown Jetton"
	}
	if metadata.Decimals == 0 {
		metadata.Decimals = 9 // Default to 9 decimals
	}

	return metadata, nil
}

// parseContentCell extracts symbol, name, and decimals from content cell
func parseContentCell(contentCell *cell.Cell, metadata *JettonMetadata) {
	if contentCell == nil {
		return
	}

	slice := contentCell.BeginParse()

	// Check if it's onchain metadata (0x00) or offchain (0x01)
	if slice.BitsLeft() < 8 {
		return
	}

	metadataType, err := slice.LoadUInt(8)
	if err != nil {
		return
	}

	if metadataType == 0x00 {
		// Onchain metadata - parse dictionary
		dict, err := slice.LoadDict(256)
		if err != nil {
			return
		}

		// Try to load symbol (key: sha256("symbol"))
		// For now, we'll skip complex dictionary parsing as it requires proper key hashing
		// This is a simplified version that works for basic cases

		if symbolVal := dict.GetByIntKey(big.NewInt(0)); symbolVal != nil {
			symbolSlice := symbolVal.BeginParse()
			if symbolSlice.BitsLeft() >= 8 {
				// Skip snake format prefix if present
				prefix, _ := symbolSlice.LoadUInt(8)
				if prefix == 0x00 {
					symbolBytes, _ := symbolSlice.LoadSlice(symbolSlice.BitsLeft())
					metadata.Symbol = string(symbolBytes)
				}
			}
		}

		// Try to load decimals
		// Using simplified parsing - in production, you'd want proper SHA256 key hashing

		if decimalsVal := dict.GetByIntKey(big.NewInt(1)); decimalsVal != nil {
			decimalsSlice := decimalsVal.BeginParse()
			if decimalsSlice.BitsLeft() >= 8 {
				// Skip snake format prefix
				prefix, _ := decimalsSlice.LoadUInt(8)
				if prefix == 0x00 {
					decimalsBytes, _ := decimalsSlice.LoadSlice(decimalsSlice.BitsLeft())
					if len(decimalsBytes) > 0 {
						// Parse decimal string
						decimalsStr := string(decimalsBytes)
						var decimals int
						fmt.Sscanf(decimalsStr, "%d", &decimals)
						if decimals > 0 {
							metadata.Decimals = decimals
						}
					}
				}
			}
		}
	}
	// For offchain metadata (0x01), we would need to fetch from URL
	// Skipping for now as it requires HTTP requests
}

// GetJettonWalletAddress calculates the jetton wallet address for a given owner
func GetJettonWalletAddress(ctx context.Context, api ton.APIClientWrapped, jettonMaster, ownerAddr *address.Address) (*address.Address, error) {
	master, err := api.CurrentMasterchainInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get master block: %w", err)
	}

	// Prepare owner address as cell
	ownerCell := cell.BeginCell().
		MustStoreAddr(ownerAddr).
		EndCell()

	// Call get_wallet_address method
	res, err := api.RunGetMethod(ctx, master, jettonMaster, "get_wallet_address", ownerCell)
	if err != nil {
		return nil, fmt.Errorf("failed to call get_wallet_address: %w", err)
	}

	// Parse response
	walletSlice, ok := res.AsTuple()[0].(*cell.Slice)
	if !ok {
		return nil, fmt.Errorf("unexpected response from get_wallet_address")
	}

	walletAddr, err := walletSlice.LoadAddr()
	if err != nil {
		return nil, fmt.Errorf("failed to parse wallet address: %w", err)
	}

	return walletAddr, nil
}

// FormatJettonAmount formats jetton amount according to decimals
func FormatJettonAmount(amount *big.Int, decimals int) string {
	if amount == nil {
		return "0"
	}

	// Convert to float with proper decimals
	divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	quotient := new(big.Float).SetInt(amount)
	divisorFloat := new(big.Float).SetInt(divisor)
	result := new(big.Float).Quo(quotient, divisorFloat)

	return result.Text('f', decimals)
}
