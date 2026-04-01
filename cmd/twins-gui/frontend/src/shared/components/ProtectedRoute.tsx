import { Outlet } from 'react-router';
import { useStore } from '@/store/useStore';

interface ProtectedRouteProps {
  requireConnection?: boolean;
}

export const ProtectedRoute: React.FC<ProtectedRouteProps> = ({
  requireConnection = false,
}) => {
  const { isConnected } = useStore((state) => state.connectionStatus);

  if (requireConnection && !isConnected) {
    return (
      <div className="flex items-center justify-center min-h-screen">
        <div className="text-center">
          <h2 className="text-2xl font-bold mb-2">Connecting to Network...</h2>
          <p className="text-gray-600">Please wait while we establish connection</p>
        </div>
      </div>
    );
  }

  return <Outlet />;
};