package blockchainlistener

import (
	"crypto/ecdsa"
	"fmt"

	"github.com/SmartMeshFoundation/Photon-Path-Finder/model"
	"github.com/SmartMeshFoundation/Photon/log"

	"github.com/SmartMeshFoundation/Photon/blockchain"
	"github.com/SmartMeshFoundation/Photon/network/helper"
	"github.com/SmartMeshFoundation/Photon/network/rpc"
	"github.com/SmartMeshFoundation/Photon/transfer"
	"github.com/SmartMeshFoundation/Photon/transfer/mediatedtransfer"
	"github.com/SmartMeshFoundation/Photon/utils"
	"github.com/ethereum/go-ethereum/common"
)

//ChainEvents block chain operations
type ChainEvents struct {
	client       *helper.SafeEthClient
	be           *blockchain.Events
	bcs          *rpc.BlockChainService
	key          *ecdsa.PrivateKey
	quitChan     chan struct{}
	stopped      bool
	TokenNetwork *TokenNetwork
	matrix       *MatrixObserver
}

// NewChainEvents create chain events
func NewChainEvents(key *ecdsa.PrivateKey, client *helper.SafeEthClient, tokenNetworkRegistryAddress common.Address) *ChainEvents { //, db *models.ModelDB
	log.Info(fmt.Sprintf("Token Network registry address=%s", tokenNetworkRegistryAddress.String()))
	bcs, err := rpc.NewBlockChainService(key, tokenNetworkRegistryAddress, client)
	if err != nil {
		log.Crit(err.Error())
	}
	registry := bcs.Registry(tokenNetworkRegistryAddress, true)
	if registry == nil {
		log.Crit("Register token network error : cannot get registry")
	}

	token2TokenNetwork := model.GetAllTokenNetworks()
	log.Trace(fmt.Sprintf("token2TokenNetwork startup=%s", utils.StringInterface(token2TokenNetwork, 2)))

	//logrus.in
	ce := &ChainEvents{
		client:       client,
		be:           blockchain.NewBlockChainEvents(client, bcs, token2TokenNetwork),
		bcs:          bcs,
		key:          key,
		quitChan:     make(chan struct{}),
		TokenNetwork: NewTokenNetwork(token2TokenNetwork),
	}
	ce.matrix = NewMatrixObserver(ce.TokenNetwork)
	return ce
}

// Start moniter blockchain
func (ce *ChainEvents) Start() error {
	ce.be.Start(ce.getLatestBlockNumber())
	go ce.loop()
	return nil
}

// Stop service
func (ce *ChainEvents) Stop() {
	ce.be.Stop()
	ce.matrix.Stop()
	close(ce.quitChan)
}

// loop loop
func (ce *ChainEvents) loop() {
	for {
		select {
		case st, ok := <-ce.be.StateChangeChannel:
			if !ok {
				log.Info("StateChangeChannel closed")
				return
			}
			ce.handleStateChange(st)
		case <-ce.quitChan:
			return
		}
	}
}

// handleStateChange 通道打开、通道关闭、通道存钱、通道取钱
func (ce *ChainEvents) handleStateChange(st transfer.StateChange) {
	switch st2 := st.(type) {
	case *transfer.BlockStateChange:
		ce.handleBlockNumber(st2.BlockNumber)
	case *mediatedtransfer.ContractNewChannelStateChange: //open channel event
		ce.handleChainChannelOpend(st2)
	case *mediatedtransfer.ContractClosedStateChange: //close channel event
		ce.handleChainChannelClosed(st2)
	case *mediatedtransfer.ContractBalanceStateChange: //deposit event
		ce.handleChainChannelDeposit(st2)
	case *mediatedtransfer.ContractChannelWithdrawStateChange: //withdaw event
		ce.handleWithdrawStateChange(st2)
	case *mediatedtransfer.ContractTokenAddedStateChange:
		//chainevent.be.TokenNetworks[st2.TokenNetworkAddress] = true
		ce.handleTokenAddedStateChange(st2)
	case *mediatedtransfer.ContractSettledStateChange:
		ce.handleChannelSettled(st2)
	case *mediatedtransfer.ContractCooperativeSettledStateChange:
		ce.handleChannelCooperativeSettled(st2)
	}
}

func (ce *ChainEvents) handleChannelSettled(st2 *mediatedtransfer.ContractSettledStateChange) {
	log.Trace(fmt.Sprintf("receive ContractSettledStateChange %s", utils.StringInterface(st2, 3)))
	err := ce.TokenNetwork.handleChannelSettled(st2.TokenNetworkAddress, st2.ChannelIdentifier)
	if err != nil {
		log.Error(fmt.Sprintf("handleChannelSettled err %s", err))
	}
}
func (ce *ChainEvents) handleChannelCooperativeSettled(st2 *mediatedtransfer.ContractCooperativeSettledStateChange) {
	log.Trace(fmt.Sprintf("receive ContractCooperativeSettledStateChange %s", utils.StringInterface(st2, 3)))
	err := ce.TokenNetwork.handleChannelCooperativeSettled(st2.TokenNetworkAddress, st2.ChannelIdentifier)
	if err != nil {
		log.Error(fmt.Sprintf("handleChannelCooperativeSettled err %s", err))
	}
}

// handleTokenAddedStateChange Token added
func (ce *ChainEvents) handleTokenAddedStateChange(st2 *mediatedtransfer.ContractTokenAddedStateChange) {
	log.Trace(fmt.Sprintf("Received TokenAddedStateChange event for token %s", st2.TokenAddress.String()))
	tokenProxy, err := ce.bcs.Token(st2.TokenAddress)
	if err != nil {
		log.Error(fmt.Sprintf("Token proxy create error %s", err))
		return
	}
	decimal, err := tokenProxy.Token.Decimals(nil)
	err = ce.TokenNetwork.handleTokenNetworkAdded(st2.TokenAddress, st2.TokenNetworkAddress, st2.BlockNumber, decimal)
	if err != nil {
		log.Error(fmt.Sprintf("handleTokenNetworkAdded err %s ", err))
	}
}

func (ce *ChainEvents) handleBlockNumber(n int64) {
	model.UpdateBlockNumber(n)
}

// handleNewChannelStateChange Open channel
func (ce *ChainEvents) handleChainChannelOpend(st2 *mediatedtransfer.ContractNewChannelStateChange) {
	tokenNetwork := st2.TokenNetworkAddress

	log.Trace(fmt.Sprintf("Received ChannelOpened event for token network %s", tokenNetwork.String()))

	channelID := st2.ChannelIdentifier.ChannelIdentifier
	participant1 := st2.Participant1
	participant2 := st2.Participant2
	log.Trace(fmt.Sprintf(fmt.Sprintf("Received ChannelOpened data: %s", utils.StringInterface(st2, 3))))
	err := ce.TokenNetwork.handleChannelOpenedEvent(tokenNetwork, channelID, participant1, participant2, st2.BlockNumber)
	if err != nil {
		log.Error(fmt.Sprintf("Handle channel open event error,err=%s", err))
	}

}

// handleDepositStateChange deposit
func (ce *ChainEvents) handleChainChannelDeposit(st2 *mediatedtransfer.ContractBalanceStateChange) {
	tokenNetwork := st2.TokenNetworkAddress

	log.Trace(fmt.Sprintf("Received ChannelDeposit event for token network %s", tokenNetwork.String()))

	channelID := st2.ChannelIdentifier
	participantAddress := st2.ParticipantAddress
	totalDeposit := st2.Balance
	log.Trace(fmt.Sprintf(fmt.Sprintf("Received ChannelDeposit data: %s", utils.StringInterface(st2, 2))))
	err := ce.TokenNetwork.handleChannelDepositEvent(tokenNetwork, channelID, participantAddress, totalDeposit)
	if err != nil {
		log.Error(fmt.Sprintf("Handle channel deposit event error,err=%s", err))
	}
}

// handleChainChannelClosed Close Channel
func (ce *ChainEvents) handleChainChannelClosed(st2 *mediatedtransfer.ContractClosedStateChange) {
	tokenNetwork := st2.TokenNetworkAddress

	log.Trace(fmt.Sprintf("Received ChannelClosed event for token network %s", tokenNetwork.String()))

	channelID := st2.ChannelIdentifier
	err := ce.TokenNetwork.handleChannelClosedEvent(tokenNetwork, channelID)
	if err != nil {
		log.Error(fmt.Sprintf("Handle channel close event error,err=%s", err))
	}
}

// handleWithdrawStateChange Withdraw
func (ce *ChainEvents) handleWithdrawStateChange(st2 *mediatedtransfer.ContractChannelWithdrawStateChange) {
	tokenNetwork := st2.TokenNetworkAddress

	log.Trace(fmt.Sprintf("Received ChannelWithdraw event for token network %s", tokenNetwork.String()))

	channelID := st2.ChannelIdentifier.ChannelIdentifier
	participant1 := st2.Participant1
	participant2 := st2.Participant2
	participant1Balance := st2.Participant1Balance
	participant2Balance := st2.Participant2Balance

	err := ce.TokenNetwork.handleChannelWithdrawEvent(tokenNetwork, channelID, participant1, participant2, participant1Balance, participant2Balance, st2.BlockNumber)
	if err != nil {
		log.Error(fmt.Sprintf("Handle channel withdaw event error,err=%s", err))
	}
}

// getLatestBlockNumber
func (ce *ChainEvents) getLatestBlockNumber() int64 {
	number := model.GetLatestBlockNumber()
	fmt.Println(number)
	return number
}
