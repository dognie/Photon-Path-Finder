package routing

import (
	"net/http"
	"github.com/ethereum/go-ethereum/common"
	"github.com/SmartMeshFoundation/SmartRaiden-Path-Finder/util"
	"math/big"
	"github.com/SmartMeshFoundation/SmartRaiden-Path-Finder/blockchainlistener"
)

//balanceProof is the json request for BalanceProof
type BalanceProof struct {
	Nonce             int64       `json:"nonce"`
	TransferredAmount *big.Int    `json:"transferred_amount"`
	ChannelID         common.Hash `json:"channel_id"`
	LocksRoot         common.Hash `json:"locksroot"`
	AdditionalHash    common.Hash `json:"additional_hash"`
	Signature         common.Hash `json:"signature"`
}

//lock is the json request for BalanceProof
type lock struct {
	LockedAmount *big.Int    `json:"locked_amount"`
	Expriation   *big.Int    `json:"expiration"`
	SecretHash   common.Hash `json:"secret_hash"`
}

//balanceProofRequest is the json request for BalanceProof
type balanceProofRequest struct {
	BalanceHash  common.Hash  `json:"balance_hash"`
	BalanceProof BalanceProof `json:"balance_proof"`
	//Locks        []lock       `json:"locks"`
	LocksAmount *big.Int `json:"locks_amount"`
}

// Balance handle the request with balance proof,implements GET and POST /balance
func UpdateBalanceProof(req *http.Request,ce blockchainlistener.ChainEvents, peerAddress string) util.JSONResponse {
	if req.Method == http.MethodPut {
		var r balanceProofRequest
		resErr := util.UnmarshalJSONRequest(req, &r)
		if resErr != nil {
			return *resErr
		}

		//validate json-input
		var partner common.Address
		//var locksAmount *big.Int
		partner, err := verifySinature(r, common.HexToAddress(peerAddress))
		if err != nil {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: err.Error(),//util.BadJSON("peerAddress must be provided"),
			}
		}

		util.GetLogger(req.Context()).WithField("balance_proof", r.BalanceHash).Info("Processing balance_proof request")

		err = ce.TokenNetwork.UpdateBalance(
			r.BalanceProof.ChannelID,
			partner,
			r.BalanceProof.Nonce,
			r.BalanceProof.TransferredAmount,
			r.LocksAmount)
		if err != nil {
			return util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: util.InvalidArgumentValue(err.Error()),
			}
		}
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: util.OkJSON("true"),
		}
	}
	return util.JSONResponse{
		Code: http.StatusMethodNotAllowed,
		JSON: util.NotFound("Bad method"),
	}
}

