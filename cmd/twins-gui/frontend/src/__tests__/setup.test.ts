import { describe, it, expect } from 'vitest';
import React from 'react';
import { render } from '@testing-library/react';

describe('Frontend Setup', () => {
  it('should have React available', () => {
    expect(React).toBeDefined();
  });

  it('should render without errors', () => {
    const TestComponent = () => React.createElement('div', null, 'Test');
    const { container } = render(React.createElement(TestComponent));
    expect(container.textContent).toBe('Test');
  });
});
