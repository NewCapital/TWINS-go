import { useTranslation } from 'react-i18next';

export const Staking: React.FC = () => {
  const { t } = useTranslation('common');

  return (
    <div>
      <h1 className="text-3xl font-bold mb-6">{t('staking.title')}</h1>
      <div className="card">
        <p className="text-gray-600">{t('staking.comingSoon')}</p>
      </div>
    </div>
  );
};