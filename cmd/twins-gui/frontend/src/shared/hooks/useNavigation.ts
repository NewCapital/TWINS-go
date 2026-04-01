import { useNavigate, useLocation } from 'react-router';
import { useCallback } from 'react';
import { ROUTES } from '@/shared/constants/routes';

export const useNavigation = () => {
  const navigate = useNavigate();
  const location = useLocation();

  const goToSend = useCallback(
    (address?: string) => {
      navigate(ROUTES.SEND, { state: { recipient: address } });
    },
    [navigate]
  );

  const goToMasternode = useCallback(
    (id: string) => {
      navigate(ROUTES.MASTERNODE_DETAIL.replace(':id', id));
    },
    [navigate]
  );

  const goBack = useCallback(() => {
    if (window.history.length > 1) {
      navigate(-1);
    } else {
      navigate(ROUTES.DASHBOARD);
    }
  }, [navigate]);

  const isActiveRoute = useCallback(
    (route: string) => {
      return location.pathname === route;
    },
    [location.pathname]
  );

  return {
    navigate,
    location,
    goToSend,
    goToMasternode,
    goBack,
    isActiveRoute,
    currentPath: location.pathname,
  };
};