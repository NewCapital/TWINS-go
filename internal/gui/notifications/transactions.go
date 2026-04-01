package notifications

import (
	"fmt"
	"sync"

	"github.com/twins-dev/twins-core/internal/gui/core"
)

// TransactionNotifier monitors and notifies about transactions
type TransactionNotifier struct {
	manager    *NotificationManager
	client     core.CoreClient
	trackedTxs map[string]*TrackedTransaction
	mu         sync.RWMutex
	done       chan struct{}
	stopOnce   sync.Once
	wg         sync.WaitGroup
}

// TrackedTransaction represents a transaction being tracked for notifications
type TrackedTransaction struct {
	TxID          string
	Type          string
	Amount        float64
	Address       string
	Confirmations int
	NotifiedAt    []int // Confirmation levels where we sent notifications
}

// NewTransactionNotifier creates a new transaction notifier
func NewTransactionNotifier(manager *NotificationManager, client core.CoreClient) *TransactionNotifier {
	return &TransactionNotifier{
		manager:    manager,
		client:     client,
		trackedTxs: make(map[string]*TrackedTransaction),
		done:       make(chan struct{}),
	}
}

// Start starts the transaction notifier by listening to core events
func (tn *TransactionNotifier) Start() {
	events := tn.client.Events()
	if events == nil {
		return // No events channel available
	}

	tn.wg.Add(1)
	go tn.eventLoop(events)
}

// Stop signals the event loop to stop and waits for cleanup (idempotent - safe to call multiple times)
func (tn *TransactionNotifier) Stop() {
	tn.stopOnce.Do(func() {
		close(tn.done)
	})
	tn.wg.Wait()
}

// eventLoop listens for core events and dispatches to handlers
func (tn *TransactionNotifier) eventLoop(events <-chan core.CoreEvent) {
	defer tn.wg.Done()

	for {
		select {
		case <-tn.done:
			return
		case event, ok := <-events:
			if !ok {
				return // Channel closed
			}
			tn.handleEvent(event)
		}
	}
}

// handleEvent dispatches events to appropriate handlers
func (tn *TransactionNotifier) handleEvent(event core.CoreEvent) {
	switch e := event.(type) {
	case core.TransactionReceivedEvent:
		tn.handleNewTransaction(e)
	case core.TransactionConfirmedEvent:
		tn.handleConfirmation(e)
	case core.StakeRewardEvent:
		tn.handleStakingReward(e)
	case core.MasternodePaymentReceivedEvent:
		tn.handleMasternodePayment(e)
	}
}

// handleNewTransaction handles new transaction events
func (tn *TransactionNotifier) handleNewTransaction(txEvent core.TransactionReceivedEvent) {
	config := tn.manager.GetConfig()

	// Check if amount meets minimum
	if txEvent.Amount < config.MinimumAmount && txEvent.Amount > -config.MinimumAmount {
		return
	}

	// Determine transaction type
	isIncoming := txEvent.Amount > 0
	if isIncoming && !config.ShowIncomingTx {
		return
	}
	if !isIncoming && !config.ShowOutgoingTx {
		return
	}

	// Track transaction
	tn.mu.Lock()
	tn.trackedTxs[txEvent.TxID] = &TrackedTransaction{
		TxID:          txEvent.TxID,
		Type:          txEvent.EventType(),
		Amount:        txEvent.Amount,
		Address:       "", // Not available in core event
		Confirmations: txEvent.Confirmations,
		NotifiedAt:    []int{},
	}
	tn.mu.Unlock()

	// Create notification
	var title, message string
	var notifType NotificationType
	var severity NotificationLevel

	if isIncoming {
		title = "Incoming Transaction"
		message = fmt.Sprintf("Received %.8f TWINS", txEvent.Amount)
		notifType = NotifTypeTransactionIn
		severity = LevelSuccess
	} else {
		title = "Outgoing Transaction"
		message = fmt.Sprintf("Sent %.8f TWINS", -txEvent.Amount)
		notifType = NotifTypeTransactionOut
		severity = LevelInfo
	}

	notif := Notification{
		Type:     notifType,
		Title:    title,
		Message:  message,
		Severity: severity,
		Data: map[string]interface{}{
			"txid":          txEvent.TxID,
			"amount":        txEvent.Amount,
			"confirmations": txEvent.Confirmations,
		},
		Actions: []NotificationAction{
			{
				Label:   "View Transaction",
				Action:  "view_transaction:" + txEvent.TxID,
				Primary: true,
			},
		},
	}

	tn.manager.Send(notif)
}

// handleConfirmation handles transaction confirmation updates
func (tn *TransactionNotifier) handleConfirmation(txEvent core.TransactionConfirmedEvent) {
	tn.mu.Lock()
	tracked, exists := tn.trackedTxs[txEvent.TxID]
	tn.mu.Unlock()

	if !exists {
		return
	}

	// Check if we should notify about this confirmation level
	if tn.shouldNotifyConfirmation(tracked, txEvent.Confirmations) {
		tn.sendConfirmationNotification(txEvent.TxID, txEvent.Confirmations)

		tn.mu.Lock()
		tracked.NotifiedAt = append(tracked.NotifiedAt, txEvent.Confirmations)
		tracked.Confirmations = txEvent.Confirmations
		tn.mu.Unlock()
	}

	// Remove from tracking after sufficient confirmations
	if txEvent.Confirmations >= 6 {
		tn.mu.Lock()
		delete(tn.trackedTxs, txEvent.TxID)
		tn.mu.Unlock()
	}
}

// shouldNotifyConfirmation checks if we should notify about confirmations
func (tn *TransactionNotifier) shouldNotifyConfirmation(tracked *TrackedTransaction, confirmations int) bool {
	config := tn.manager.GetConfig()

	if confirmations < config.ConfirmationsRequired {
		return false
	}

	// Notify at specific milestones: 1, 3, 6
	milestones := []int{1, 3, 6}

	for _, milestone := range milestones {
		if confirmations >= milestone {
			// Check if already notified at this level
			alreadyNotified := false
			for _, notifiedAt := range tracked.NotifiedAt {
				if notifiedAt == milestone {
					alreadyNotified = true
					break
				}
			}

			if !alreadyNotified && confirmations == milestone {
				return true
			}
		}
	}

	return false
}

// sendConfirmationNotification sends a confirmation notification
func (tn *TransactionNotifier) sendConfirmationNotification(txid string, confirmations int) {
	message := fmt.Sprintf("Transaction has %d confirmation%s",
		confirmations,
		map[bool]string{true: "s", false: ""}[confirmations != 1],
	)

	notif := Notification{
		Type:     NotifTypeConfirmation,
		Title:    "Transaction Confirmed",
		Message:  message,
		Severity: LevelInfo,
		Data: map[string]interface{}{
			"txid":          txid,
			"confirmations": confirmations,
		},
	}

	tn.manager.Send(notif)
}

// handleStakingReward handles staking reward notifications
func (tn *TransactionNotifier) handleStakingReward(stakingEvent core.StakeRewardEvent) {
	config := tn.manager.GetConfig()

	if !config.ShowStakingRewards {
		return
	}

	if stakingEvent.Amount < config.MinimumAmount {
		return
	}

	notif := Notification{
		Type:     NotifTypeStakeReward,
		Title:    "Staking Reward",
		Message:  fmt.Sprintf("Earned %.8f TWINS from staking", stakingEvent.Amount),
		Severity: LevelSuccess,
		Data: map[string]interface{}{
			"txid":         stakingEvent.TxID,
			"amount":       stakingEvent.Amount,
			"block_height": stakingEvent.Height,
		},
		Actions: []NotificationAction{
			{
				Label:   "View Details",
				Action:  "view_staking_stats",
				Primary: true,
			},
		},
	}

	tn.manager.Send(notif)
}

// handleMasternodePayment handles masternode payment notifications
func (tn *TransactionNotifier) handleMasternodePayment(mnEvent core.MasternodePaymentReceivedEvent) {
	config := tn.manager.GetConfig()

	if !config.ShowMasternodePayments {
		return
	}

	if mnEvent.Amount < config.MinimumAmount {
		return
	}

	notif := Notification{
		Type:     NotifTypeMasternodePay,
		Title:    "Masternode Payment",
		Message:  fmt.Sprintf("Received %.8f TWINS from masternode %s", mnEvent.Amount, mnEvent.Alias),
		Severity: LevelSuccess,
		Data: map[string]interface{}{
			"alias":  mnEvent.Alias,
			"txid":   mnEvent.TxID,
			"amount": mnEvent.Amount,
		},
		Actions: []NotificationAction{
			{
				Label:   "View Masternode",
				Action:  "view_masternode:" + mnEvent.Alias,
				Primary: true,
			},
		},
	}

	tn.manager.Send(notif)
}

// GetTrackedCount returns the number of tracked transactions
func (tn *TransactionNotifier) GetTrackedCount() int {
	tn.mu.RLock()
	defer tn.mu.RUnlock()
	return len(tn.trackedTxs)
}
