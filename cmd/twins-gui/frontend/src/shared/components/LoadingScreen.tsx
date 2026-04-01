import { useTranslation } from 'react-i18next';

export const LoadingScreen: React.FC = () => {
  const { t } = useTranslation('common');

  return (
    <div className="flex items-center justify-center min-h-screen">
      <div className="text-center">
        <div className="inline-block animate-spin rounded-full h-12 w-12 border-b-2 border-twins-600"></div>
        <p className="mt-4 text-gray-600">{t('loading.default')}</p>
      </div>
    </div>
  );
};