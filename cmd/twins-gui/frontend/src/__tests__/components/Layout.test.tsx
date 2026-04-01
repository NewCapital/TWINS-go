import { render, screen } from '@testing-library/react';
import { Layout } from '@/shared/components/Layout';

describe('Layout', () => {
  it('renders children correctly', () => {
    render(
      <Layout>
        <div>Test Content</div>
      </Layout>
    );

    expect(screen.getByText('Test Content')).toBeInTheDocument();
  });
});
