package events

import (
	"sync"

	"github.com/twins-dev/twins-core/internal/gui/core"
)

// Event type constants for frontend bridge
const (
	EventNewBlock          = "new_block"
	EventNewTransaction    = "new_transaction"
	EventBalanceChanged    = "balance_changed"
	EventStakeReward       = "stake_reward"
	EventMasternodePayment = "masternode_payment"
	EventWalletLocked      = "wallet_locked"
	EventWalletUnlocked    = "wallet_unlocked"
)

// EventIntegration connects core events to the frontend bridge
type EventIntegration struct {
	bridge   *EventBridge
	client   core.CoreClient
	done     chan struct{}
	stopOnce sync.Once
}

// NewEventIntegration creates a new event integration
func NewEventIntegration(bridge *EventBridge, client core.CoreClient) *EventIntegration {
	return &EventIntegration{
		bridge: bridge,
		client: client,
		done:   make(chan struct{}),
	}
}

// Start begins listening for core events and forwarding to frontend
func (ei *EventIntegration) Start() error {
	events := ei.client.Events()
	if events == nil {
		return nil // No events channel available
	}

	go ei.eventLoop(events)
	return nil
}

// Stop signals the event loop to stop (idempotent - safe to call multiple times)
func (ei *EventIntegration) Stop() {
	ei.stopOnce.Do(func() {
		close(ei.done)
	})
}

// eventLoop listens for core events and dispatches them to handlers
func (ei *EventIntegration) eventLoop(events <-chan core.CoreEvent) {
	for {
		select {
		case <-ei.done:
			return
		case event, ok := <-events:
			if !ok {
				return // Channel closed
			}
			ei.handleEvent(event)
		}
	}
}

// handleEvent dispatches events to appropriate handlers based on type
func (ei *EventIntegration) handleEvent(event core.CoreEvent) {
	switch e := event.(type) {
	case core.BlockConnectedEvent:
		ei.bridge.EmitJSON(EventNewBlock, e)
	case core.TransactionReceivedEvent:
		ei.bridge.EmitJSON(EventNewTransaction, e)
	case core.BalanceChangedEvent:
		ei.bridge.EmitJSON(EventBalanceChanged, e)
	case core.StakeRewardEvent:
		ei.bridge.EmitJSON(EventStakeReward, e)
	case core.MasternodePaymentReceivedEvent:
		ei.bridge.EmitJSON(EventMasternodePayment, e)
	case core.WalletLockedEvent:
		ei.bridge.Emit(EventWalletLocked, nil)
	case core.WalletUnlockedEvent:
		ei.bridge.Emit(EventWalletUnlocked, nil)
	}
}
