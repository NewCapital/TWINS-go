import { Transaction } from '@/shared/types/wallet.types';

interface TransactionListProps {
  transactions: Transaction[];
}

export const TransactionList: React.FC<TransactionListProps> = ({ transactions }) => {
  if (transactions.length === 0) {
    return (
      <div className="empty-transaction-list">
        {/* Empty state - no placeholder text as per Qt wallet */}
      </div>
    );
  }

  return (
    <div className="space-y-2">
      {transactions.map((tx) => (
        <div key={`${tx.txid}:${tx.vout}`} className="flex items-center justify-between p-3 border rounded-lg hover:bg-gray-50">
          <div className="flex items-center gap-3">
            <div className={`w-10 h-10 rounded-full flex items-center justify-center ${
              tx.type === 'receive' ? 'bg-green-100' : 'bg-red-100'
            }`}>
              <span className={tx.type === 'receive' ? 'text-green-600' : 'text-red-600'}>
                {tx.type === 'receive' ? '↓' : '↑'}
              </span>
            </div>
            <div>
              <div className="font-medium capitalize">{tx.type}</div>
              <div className="text-sm text-gray-500">
                {tx.address.substring(0, 8)}...{tx.address.substring(tx.address.length - 6)}
              </div>
            </div>
          </div>
          <div className="text-right">
            <div className={`font-mono font-medium ${
              tx.type === 'receive' ? 'text-green-600' : 'text-red-600'
            }`}>
              {tx.type === 'receive' ? '+' : '-'}{tx.amount.toFixed(8)} TWINS
            </div>
            <div className="text-sm text-gray-500">{tx.confirmations} confirmations</div>
          </div>
        </div>
      ))}
    </div>
  );
};