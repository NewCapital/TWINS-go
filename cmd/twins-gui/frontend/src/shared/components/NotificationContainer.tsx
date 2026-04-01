import { useNotifications } from '@/store/useStore';

const borderColors = {
  success: '#4caf50',
  error: '#d32f2f',
  warning: '#f8d488',
  info: '#3498db',
} as const;

export const NotificationContainer: React.FC = () => {
  const { notifications, removeNotification } = useNotifications();

  if (notifications.length === 0) return null;

  return (
    <div style={{ position: 'fixed', bottom: 'calc(var(--qt-statusbar-height) + 12px)', right: 16, zIndex: 51, display: 'flex', flexDirection: 'column', gap: 8 }}>
      {notifications.map((notification) => (
        <div
          key={notification.id}
          style={{
            padding: '12px 16px',
            borderRadius: 6,
            maxWidth: 400,
            backgroundColor: 'var(--qt-bg-primary)',
            borderLeft: `4px solid ${borderColors[notification.type] || borderColors.info}`,
            boxShadow: '0 4px 12px rgba(0, 0, 0, 0.4)',
          }}
        >
          <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
            <div>
              <h4 style={{ fontWeight: 500, color: 'var(--qt-text-primary)', fontSize: 13 }}>
                {notification.title}
              </h4>
              {notification.message && (
                <p style={{ fontSize: 12, color: 'var(--qt-text-secondary)', marginTop: 4 }}>
                  {notification.message}
                </p>
              )}
            </div>
            <button
              onClick={() => removeNotification(notification.id)}
              style={{
                background: 'none',
                border: 'none',
                color: 'var(--qt-text-secondary)',
                cursor: 'pointer',
                fontSize: 18,
                lineHeight: 1,
                padding: 4,
                borderRadius: 4,
                flexShrink: 0,
              }}
              onMouseEnter={(e) => { e.currentTarget.style.color = 'var(--qt-text-primary)'; }}
              onMouseLeave={(e) => { e.currentTarget.style.color = 'var(--qt-text-secondary)'; }}
            >
              ×
            </button>
          </div>
        </div>
      ))}
    </div>
  );
};