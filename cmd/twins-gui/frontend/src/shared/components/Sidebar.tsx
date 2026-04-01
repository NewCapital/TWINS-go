import { NavLink, useLocation } from 'react-router';
import { ROUTES } from '@/shared/constants/routes';
import { useState, useEffect } from 'react';
import { useStore } from '@/store/useStore';
import { useTranslation } from 'react-i18next';
import { GetSettingBool } from '@wailsjs/go/main/App';
import '@/styles/qt-theme.css';

interface NavItem {
  path: string;
  labelKey: string; // i18n key for translation
  iconPath: string;
}

// Match Qt wallet navigation structure using original Qt icons
// Labels use i18n keys from common.json navigation namespace
const navItems: NavItem[] = [
  {
    path: ROUTES.DASHBOARD,
    labelKey: 'navigation.overview',
    iconPath: '/icons/overview.png',
  },
  {
    path: ROUTES.SEND,
    labelKey: 'navigation.send',
    iconPath: '/icons/send.png',
  },
  {
    path: ROUTES.RECEIVE,
    labelKey: 'navigation.receive',
    iconPath: '/icons/receive.png',
  },
  {
    path: ROUTES.TRANSACTIONS,
    labelKey: 'navigation.transactions',
    iconPath: '/icons/history.png',
  },
  {
    path: ROUTES.MASTERNODES,
    labelKey: 'navigation.masternodes',
    iconPath: '/icons/masternodes.png',
  },
  {
    path: ROUTES.EXPLORER,
    labelKey: 'navigation.explorer',
    iconPath: '/icons/masternodes.png', // TODO: Add dedicated explorer icon
  },
];

export const Sidebar: React.FC = () => {
  const location = useLocation();
  const [hoveredItem, setHoveredItem] = useState<string | null>(null);
  const [showMasternodesTab, setShowMasternodesTab] = useState(true);
  const setExplorerView = useStore((state) => state.setView);
  const { t } = useTranslation('common');

  // Read fShowMasternodesTab on mount (requiresRestart: true, so once is sufficient)
  useEffect(() => {
    GetSettingBool('fShowMasternodesTab').then(setShowMasternodesTab).catch(() => {});
  }, []);

  return (
    <div
      className="flex flex-col"
      style={{
        width: '90px',
        backgroundColor: '#000000', // Pure black like Qt
        borderRight: '1px solid #2a2a2a',
        height: '100vh',
      }}
    >

      {/* Navigation Items */}
      <nav className="flex-1 py-2">
        {navItems.filter((item) => item.path !== ROUTES.MASTERNODES || showMasternodesTab).map((item) => {
          const isActive = location.pathname === item.path ||
                          (item.path === ROUTES.DASHBOARD && location.pathname === '/');
          const label = t(item.labelKey);

          // Handle Explorer click - reset to blocks view
          const handleClick = () => {
            if (item.path === ROUTES.EXPLORER) {
              setExplorerView('blocks');
            }
          };

          return (
            <NavLink
              key={item.path}
              to={item.path}
              className="group relative"
              style={{ textDecoration: 'none' }}
              onClick={handleClick}
            >
              <div
                className="qt-nav-item flex flex-col items-center justify-center"
                style={{
                  height: '60px',
                  backgroundColor: isActive ? 'rgba(255, 255, 255, 0.05)' : 'transparent',
                  borderLeft: isActive ? '3px solid #27ae60' : '3px solid transparent',
                  cursor: 'pointer',
                  transition: 'all 0.2s cubic-bezier(0.4, 0, 0.2, 1)',
                }}
                onMouseEnter={(e) => {
                  setHoveredItem(item.labelKey);
                  if (!isActive) {
                    e.currentTarget.style.backgroundColor = 'rgba(255, 255, 255, 0.03)';
                  }
                }}
                onMouseLeave={(e) => {
                  setHoveredItem(null);
                  if (!isActive) {
                    e.currentTarget.style.backgroundColor = 'transparent';
                  }
                }}
              >
                <img
                  src={item.iconPath}
                  alt={label}
                  className="qt-icon-hover"
                  style={{
                    width: '36px',
                    height: '36px',
                    objectFit: 'contain',
                    opacity: isActive ? 1 : 0.9,
                    marginBottom: '2px',
                    transition: 'all 0.2s ease',
                    filter: isActive ? 'brightness(1.1)' : 'brightness(1)',
                  }}
                />
                <span
                  className="text-xs"
                  style={{
                    color: isActive ? '#ffffff' : '#969696',
                    fontSize: '10px',
                    transition: 'color 0.2s ease',
                  }}
                >
                  {label}
                </span>
                {/* Tooltip */}
                {hoveredItem === item.labelKey && (
                  <div
                    className="qt-tooltip visible"
                    style={{
                      left: '100%',
                      top: '50%',
                      transform: 'translateY(-50%)',
                      marginLeft: '10px',
                    }}
                  >
                    {label}
                  </div>
                )}
              </div>
            </NavLink>
          );
        })}
      </nav>
    </div>
  );
};