import { lazy, Suspense } from 'react';
import { createBrowserRouter, Navigate, useRouteError } from 'react-router';
import { ROUTES } from '@/shared/constants/routes';
import { MainLayout } from '@/shared/components/MainLayout';
import { ProtectedRoute } from '@/shared/components/ProtectedRoute';
import { LoadingScreen } from '@/shared/components/LoadingScreen';

// Retry wrapper for lazy imports - handles module load failures after app restart
function lazyWithRetry(
  factory: () => Promise<{ default: React.ComponentType }>,
): React.LazyExoticComponent<React.ComponentType> {
  return lazy(() =>
    factory().catch(() =>
      new Promise<{ default: React.ComponentType }>((resolve) => {
        setTimeout(() => resolve(factory()), 1000);
      }),
    ),
  );
}

// Error boundary for route-level errors (e.g. module import failures after restart)
function RouteErrorBoundary() {
  const error = useRouteError();
  const isModuleError =
    error instanceof TypeError && String(error.message).toLowerCase().includes('module');

  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        justifyContent: 'center',
        height: '100%',
        gap: '16px',
        color: '#ddd',
        padding: '32px',
        textAlign: 'center',
      }}
    >
      <p style={{ fontSize: '14px', color: '#aaa' }}>
        {isModuleError
          ? 'The application restarted and needs to reload.'
          : 'Something went wrong loading this page.'}
      </p>
      <button
        onClick={() => window.location.reload()}
        style={{
          padding: '8px 24px',
          backgroundColor: '#4a4a4a',
          color: '#ddd',
          border: '1px solid #666',
          borderRadius: '4px',
          cursor: 'pointer',
          fontSize: '13px',
        }}
      >
        Reload
      </button>
    </div>
  );
}

// Lazy load pages for code splitting (with retry for restart resilience)
const Dashboard = lazyWithRetry(() =>
  import('@/features/wallet/pages/Dashboard').then((m) => ({ default: m.Dashboard }))
);
const Send = lazyWithRetry(() =>
  import('@/features/wallet/pages/Send').then((m) => ({ default: m.Send }))
);
const Receive = lazyWithRetry(() =>
  import('@/features/wallet/pages/Receive').then((m) => ({ default: m.Receive }))
);
const Transactions = lazyWithRetry(() =>
  import('@/features/wallet/pages/Transactions').then((m) => ({ default: m.Transactions }))
);
const Masternodes = lazyWithRetry(() =>
  import('@/features/masternode/pages/MasternodesPage').then((m) => ({ default: m.Masternodes }))
);
const Staking = lazyWithRetry(() =>
  import('@/features/staking/pages/Staking').then((m) => ({ default: m.Staking }))
);
const Explorer = lazyWithRetry(() =>
  import('@/features/explorer/pages/ExplorerPage').then((m) => ({ default: m.Explorer }))
);

// Wrapper for lazy loaded components
const LazyPage = ({ Component }: { Component: React.LazyExoticComponent<React.ComponentType> }) => (
  <Suspense fallback={<LoadingScreen />}>
    <Component />
  </Suspense>
);

export const router = createBrowserRouter([
  // Main app routes (with layout)
  {
    element: <MainLayout />,
    errorElement: <RouteErrorBoundary />,
    children: [
      {
        element: <ProtectedRoute />,
        errorElement: <RouteErrorBoundary />,
        children: [
          {
            path: ROUTES.DASHBOARD,
            element: <LazyPage Component={Dashboard} />,
          },
          {
            path: ROUTES.WALLET,
            element: <Navigate to={ROUTES.TRANSACTIONS} replace />,
          },
          {
            path: ROUTES.SEND,
            element: <LazyPage Component={Send} />,
          },
          {
            path: ROUTES.RECEIVE,
            element: <LazyPage Component={Receive} />,
          },
          {
            path: ROUTES.TRANSACTIONS,
            element: <LazyPage Component={Transactions} />,
          },
          {
            path: ROUTES.MASTERNODES,
            element: <LazyPage Component={Masternodes} />,
          },
          {
            path: ROUTES.EXPLORER,
            element: <LazyPage Component={Explorer} />,
          },
          {
            path: ROUTES.STAKING,
            element: <LazyPage Component={Staking} />,
          },
        ],
      },
    ],
  },

  // Catch all - redirect to dashboard
  {
    path: '*',
    element: <Navigate to={ROUTES.DASHBOARD} replace />,
  },
]);