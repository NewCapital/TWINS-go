import React from 'react';
import { DaemonCategorySection, type DaemonCategorySectionProps } from './DaemonCategorySection';

type DaemonTabProps = Omit<DaemonCategorySectionProps, 'filterCategories'>;

const DAEMON_CATEGORIES = ['network', 'staking', 'wallet', 'rpc', 'masternode', 'logging', 'sync'];

export const DaemonTab: React.FC<DaemonTabProps> = (props) => (
  <DaemonCategorySection filterCategories={DAEMON_CATEGORIES} {...props} />
);

export default DaemonTab;
