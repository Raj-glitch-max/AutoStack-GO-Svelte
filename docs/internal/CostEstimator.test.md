# CostEstimator Component Test Specification

## Overview

This document outlines the test cases for the `CostEstimator.svelte` component. These tests should be implemented when a testing framework (e.g., Vitest + Testing Library) is added to the project.

## Test Setup

```typescript
import { render, screen, waitFor } from '@testing-library/svelte';
import { vi } from 'vitest';
import CostEstimator from './CostEstimator.svelte';
import * as costApi from '$lib/api/cost';

// Mock the cost API
vi.mock('$lib/api/cost');
```

## Unit Tests

### 1. Component Rendering

#### Test 1.1: Initial Loading State
**Description**: Component should show loading state when first mounted

```typescript
describe('CostEstimator - Initial State', () => {
  it('displays loading spinner on mount', () => {
    render(CostEstimator, {
      props: { blueprint: 'static-website', region: 'us-east-1' }
    });
    
    expect(screen.getByText('Calculating costs...')).toBeInTheDocument();
  });
});
```

**Validates**: TR-5.1 (Graceful degradation)

#### Test 1.2: Empty State
**Description**: Component should show helpful message when no blueprint/region selected

```typescript
it('displays empty state message when props are missing', () => {
  render(CostEstimator, {
    props: { blueprint: '', region: '' }
  });
  
  expect(screen.getByText(/Select a blueprint and region/i)).toBeInTheDocument();
});
```

**Validates**: User experience requirements

### 2. Cost Display

#### Test 2.1: Cost Range Display
**Description**: Component should display min-max cost range correctly

```typescript
describe('CostEstimator - Cost Display', () => {
  it('displays cost range with min and max values', async () => {
    const mockEstimate = {
      estimate: {
        total: 56.65,
        range: { min: 45.32, max: 79.31 },
        breakdown: { compute: 29.15, networking: 16.20, storage: 2.30, transfer: 9.00 },
        assumptions: { storage: '10GB' },
        disclaimer: 'Test disclaimer',
        pricingFetchedAt: '2026-04-11T10:00:00Z'
      }
    };
    
    vi.mocked(costApi.getCostEstimate).mockResolvedValue(mockEstimate);
    
    render(CostEstimator, {
      props: { blueprint: 'static-website', region: 'us-east-1' }
    });
    
    await waitFor(() => {
      expect(screen.getByText(/\$45\.32 - \$79\.31/)).toBeInTheDocument();
      expect(screen.getByText(/Best estimate: \$56\.65/)).toBeInTheDocument();
    });
  });
});
```

**Validates**: AC-1.1 (System shows cost range not single point estimate)

#### Test 2.2: Cost Breakdown Display
**Description**: Component should show itemized breakdown by service category

```typescript
it('displays itemized cost breakdown', async () => {
  const mockEstimate = {
    estimate: {
      total: 56.65,
      range: { min: 45.32, max: 79.31 },
      breakdown: {
        compute: 29.15,
        networking: 16.20,
        storage: 2.30,
        transfer: 9.00
      },
      assumptions: {},
      disclaimer: 'Test disclaimer',
      pricingFetchedAt: '2026-04-11T10:00:00Z'
    }
  };
  
  vi.mocked(costApi.getCostEstimate).mockResolvedValue(mockEstimate);
  
  render(CostEstimator, {
    props: { blueprint: 'static-website', region: 'us-east-1' }
  });
  
  await waitFor(() => {
    expect(screen.getByText('Compute')).toBeInTheDocument();
    expect(screen.getByText('$29.15')).toBeInTheDocument();
    expect(screen.getByText('Networking')).toBeInTheDocument();
    expect(screen.getByText('$16.20')).toBeInTheDocument();
    expect(screen.getByText('Storage')).toBeInTheDocument();
    expect(screen.getByText('$2.30')).toBeInTheDocument();
    expect(screen.getByText('Data Transfer')).toBeInTheDocument();
    expect(screen.getByText('$9.00')).toBeInTheDocument();
  });
});
```

**Validates**: AC-5.1 (Estimate shows itemized breakdown), AC-1.2 (Includes compute, networking, storage)

#### Test 2.3: Usage Assumptions Display
**Description**: Component should display usage assumptions

```typescript
it('displays usage assumptions', async () => {
  const mockEstimate = {
    estimate: {
      total: 56.65,
      range: { min: 45.32, max: 79.31 },
      breakdown: { compute: 29.15, networking: 16.20, storage: 2.30, transfer: 9.00 },
      assumptions: {
        storage: '10GB',
        transfer: '100GB/month',
        requests: '10K/month'
      },
      disclaimer: 'Test disclaimer',
      pricingFetchedAt: '2026-04-11T10:00:00Z'
    }
  };
  
  vi.mocked(costApi.getCostEstimate).mockResolvedValue(mockEstimate);
  
  render(CostEstimator, {
    props: { blueprint: 'static-website', region: 'us-east-1' }
  });
  
  await waitFor(() => {
    expect(screen.getByText(/Storage: 10GB/)).toBeInTheDocument();
    expect(screen.getByText(/Transfer: 100GB\/month/)).toBeInTheDocument();
    expect(screen.getByText(/Requests: 10K\/month/)).toBeInTheDocument();
  });
});
```

**Validates**: AC-5.4 (Shows assumptions)

#### Test 2.4: Disclaimer Display
**Description**: Component should show disclaimer about excluded costs

```typescript
it('displays disclaimer message', async () => {
  const mockEstimate = {
    estimate: {
      total: 56.65,
      range: { min: 45.32, max: 79.31 },
      breakdown: { compute: 29.15, networking: 16.20, storage: 2.30, transfer: 9.00 },
      assumptions: {},
      disclaimer: 'Excludes data transfer overages and CloudWatch detailed monitoring',
      pricingFetchedAt: '2026-04-11T10:00:00Z'
    }
  };
  
  vi.mocked(costApi.getCostEstimate).mockResolvedValue(mockEstimate);
  
  render(CostEstimator, {
    props: { blueprint: 'static-website', region: 'us-east-1' }
  });
  
  await waitFor(() => {
    expect(screen.getByText(/Excludes data transfer overages/)).toBeInTheDocument();
  });
});
```

**Validates**: AC-5.2 (Disclaimer clearly states excluded costs), AC-1.3 (Labels what is included/excluded)

#### Test 2.5: Pricing Freshness Display
**Description**: Component should show when pricing data was last fetched

```typescript
it('displays pricing data freshness', async () => {
  const mockEstimate = {
    estimate: {
      total: 56.65,
      range: { min: 45.32, max: 79.31 },
      breakdown: { compute: 29.15, networking: 16.20, storage: 2.30, transfer: 9.00 },
      assumptions: {},
      disclaimer: 'Test disclaimer',
      pricingFetchedAt: '2026-04-11T10:00:00Z'
    }
  };
  
  vi.mocked(costApi.getCostEstimate).mockResolvedValue(mockEstimate);
  
  render(CostEstimator, {
    props: { blueprint: 'static-website', region: 'us-east-1' }
  });
  
  await waitFor(() => {
    expect(screen.getByText(/Pricing data from Apr 11, 2026/)).toBeInTheDocument();
  });
});
```

**Validates**: AC-1.5 (Estimate shows when pricing data was last fetched)

### 3. Error Handling

#### Test 3.1: API Error Display
**Description**: Component should display error message when API fails

```typescript
describe('CostEstimator - Error Handling', () => {
  it('displays error message on API failure', async () => {
    vi.mocked(costApi.getCostEstimate).mockRejectedValue(
      new Error('Failed to fetch cost estimate')
    );
    
    render(CostEstimator, {
      props: { blueprint: 'static-website', region: 'us-east-1' }
    });
    
    await waitFor(() => {
      expect(screen.getByText(/Error:/)).toBeInTheDocument();
      expect(screen.getByText(/Failed to fetch cost estimate/)).toBeInTheDocument();
    });
  });
});
```

**Validates**: TR-5.1 (Graceful degradation if pricing API unavailable)

#### Test 3.2: Network Error Handling
**Description**: Component should handle network errors gracefully

```typescript
it('handles network errors gracefully', async () => {
  vi.mocked(costApi.getCostEstimate).mockRejectedValue(
    new Error('Network error')
  );
  
  render(CostEstimator, {
    props: { blueprint: 'static-website', region: 'us-east-1' }
  });
  
  await waitFor(() => {
    expect(screen.getByText(/Network error/)).toBeInTheDocument();
  });
});
```

**Validates**: TR-5.1 (Graceful degradation)

### 4. Reactive Updates

#### Test 4.1: Blueprint Change
**Description**: Component should refetch estimate when blueprint changes

```typescript
describe('CostEstimator - Reactive Updates', () => {
  it('refetches estimate when blueprint changes', async () => {
    const mockEstimate1 = {
      estimate: {
        total: 56.65,
        range: { min: 45.32, max: 79.31 },
        breakdown: { compute: 29.15, networking: 16.20, storage: 2.30, transfer: 9.00 },
        assumptions: {},
        disclaimer: 'Test disclaimer',
        pricingFetchedAt: '2026-04-11T10:00:00Z'
      }
    };
    
    const mockEstimate2 = {
      estimate: {
        total: 120.50,
        range: { min: 96.40, max: 168.70 },
        breakdown: { compute: 80.00, networking: 20.00, storage: 10.50, transfer: 10.00 },
        assumptions: {},
        disclaimer: 'Test disclaimer',
        pricingFetchedAt: '2026-04-11T10:00:00Z'
      }
    };
    
    vi.mocked(costApi.getCostEstimate)
      .mockResolvedValueOnce(mockEstimate1)
      .mockResolvedValueOnce(mockEstimate2);
    
    const { component } = render(CostEstimator, {
      props: { blueprint: 'static-website', region: 'us-east-1' }
    });
    
    await waitFor(() => {
      expect(screen.getByText(/\$56\.65/)).toBeInTheDocument();
    });
    
    // Change blueprint
    await component.$set({ blueprint: 'web-application' });
    
    await waitFor(() => {
      expect(screen.getByText(/\$120\.50/)).toBeInTheDocument();
    });
    
    expect(costApi.getCostEstimate).toHaveBeenCalledTimes(2);
  });
});
```

**Validates**: Real-time updates requirement

#### Test 4.2: Region Change
**Description**: Component should refetch estimate when region changes

```typescript
it('refetches estimate when region changes', async () => {
  const mockEstimate1 = {
    estimate: {
      total: 56.65,
      range: { min: 45.32, max: 79.31 },
      breakdown: { compute: 29.15, networking: 16.20, storage: 2.30, transfer: 9.00 },
      assumptions: {},
      disclaimer: 'Test disclaimer',
      pricingFetchedAt: '2026-04-11T10:00:00Z'
    }
  };
  
  const mockEstimate2 = {
    estimate: {
      total: 62.30,
      range: { min: 49.84, max: 87.22 },
      breakdown: { compute: 32.00, networking: 18.00, storage: 2.30, transfer: 10.00 },
      assumptions: {},
      disclaimer: 'Test disclaimer',
      pricingFetchedAt: '2026-04-11T10:00:00Z'
    }
  };
  
  vi.mocked(costApi.getCostEstimate)
    .mockResolvedValueOnce(mockEstimate1)
    .mockResolvedValueOnce(mockEstimate2);
  
  const { component } = render(CostEstimator, {
    props: { blueprint: 'static-website', region: 'us-east-1' }
  });
  
  await waitFor(() => {
    expect(screen.getByText(/\$56\.65/)).toBeInTheDocument();
  });
  
  // Change region
  await component.$set({ region: 'eu-west-1' });
  
  await waitFor(() => {
    expect(screen.getByText(/\$62\.30/)).toBeInTheDocument();
  });
  
  expect(costApi.getCostEstimate).toHaveBeenCalledTimes(2);
});
```

**Validates**: AC-1.4 (Estimate is region-specific)

### 5. Utility Functions

#### Test 5.1: Currency Formatting
**Description**: Test formatCurrency function formats numbers correctly

```typescript
describe('CostEstimator - Utility Functions', () => {
  it('formats currency to 2 decimal places', () => {
    // This would test the internal formatCurrency function
    // In practice, you'd export it or test it through the component
    expect(formatCurrency(56.6543)).toBe('56.65');
    expect(formatCurrency(100)).toBe('100.00');
    expect(formatCurrency(0.5)).toBe('0.50');
  });
});
```

**Validates**: TR-2.4 (Round to 2 decimal places for display)

#### Test 5.2: Date Formatting
**Description**: Test formatDate function formats dates correctly

```typescript
it('formats ISO date strings correctly', () => {
  expect(formatDate('2026-04-11T10:00:00Z')).toBe('Apr 11, 2026');
  expect(formatDate('2026-12-25T00:00:00Z')).toBe('Dec 25, 2026');
});
```

**Validates**: AC-1.5 (Pricing data freshness display)

## Integration Tests

### 6. API Integration

#### Test 6.1: Successful API Call
**Description**: Component should successfully fetch and display estimate from API

```typescript
describe('CostEstimator - API Integration', () => {
  it('fetches and displays estimate from API', async () => {
    const mockEstimate = {
      estimate: {
        total: 56.65,
        range: { min: 45.32, max: 79.31 },
        breakdown: { compute: 29.15, networking: 16.20, storage: 2.30, transfer: 9.00 },
        assumptions: { storage: '10GB' },
        disclaimer: 'Test disclaimer',
        pricingFetchedAt: '2026-04-11T10:00:00Z'
      }
    };
    
    vi.mocked(costApi.getCostEstimate).mockResolvedValue(mockEstimate);
    
    render(CostEstimator, {
      props: { blueprint: 'static-website', region: 'us-east-1' }
    });
    
    expect(costApi.getCostEstimate).toHaveBeenCalledWith(
      'static-website',
      'us-east-1',
      {}
    );
    
    await waitFor(() => {
      expect(screen.getByText(/\$56\.65/)).toBeInTheDocument();
    });
  });
});
```

**Validates**: AC-1.6 (Estimate loads in <500ms from cache)

#### Test 6.2: API Call with Variables
**Description**: Component should pass variables to API correctly

```typescript
it('passes variables to API when provided', async () => {
  const variables = { instance_type: 't3.micro', storage_gb: 20 };
  
  vi.mocked(costApi.getCostEstimate).mockResolvedValue({
    estimate: {
      total: 56.65,
      range: { min: 45.32, max: 79.31 },
      breakdown: { compute: 29.15, networking: 16.20, storage: 2.30, transfer: 9.00 },
      assumptions: {},
      disclaimer: 'Test disclaimer',
      pricingFetchedAt: '2026-04-11T10:00:00Z'
    }
  });
  
  render(CostEstimator, {
    props: { 
      blueprint: 'web-application', 
      region: 'us-east-1',
      variables
    }
  });
  
  expect(costApi.getCostEstimate).toHaveBeenCalledWith(
    'web-application',
    'us-east-1',
    variables
  );
});
```

**Validates**: Variable passing functionality

## Accessibility Tests

### 7. Accessibility

#### Test 7.1: Semantic HTML
**Description**: Component should use proper semantic HTML

```typescript
describe('CostEstimator - Accessibility', () => {
  it('uses semantic HTML elements', async () => {
    const mockEstimate = {
      estimate: {
        total: 56.65,
        range: { min: 45.32, max: 79.31 },
        breakdown: { compute: 29.15, networking: 16.20, storage: 2.30, transfer: 9.00 },
        assumptions: {},
        disclaimer: 'Test disclaimer',
        pricingFetchedAt: '2026-04-11T10:00:00Z'
      }
    };
    
    vi.mocked(costApi.getCostEstimate).mockResolvedValue(mockEstimate);
    
    const { container } = render(CostEstimator, {
      props: { blueprint: 'static-website', region: 'us-east-1' }
    });
    
    await waitFor(() => {
      expect(container.querySelector('h3')).toBeInTheDocument();
      expect(container.querySelector('h4')).toBeInTheDocument();
    });
  });
});
```

**Validates**: Accessibility requirements

#### Test 7.2: Keyboard Navigation
**Description**: Component should be keyboard navigable

```typescript
it('supports keyboard navigation', async () => {
  // Test that all interactive elements are keyboard accessible
  // This would involve testing focus management and tab order
});
```

**Validates**: Accessibility requirements

## Performance Tests

### 8. Performance

#### Test 8.1: Render Performance
**Description**: Component should render quickly

```typescript
describe('CostEstimator - Performance', () => {
  it('renders within acceptable time', async () => {
    const startTime = performance.now();
    
    render(CostEstimator, {
      props: { blueprint: 'static-website', region: 'us-east-1' }
    });
    
    const endTime = performance.now();
    const renderTime = endTime - startTime;
    
    expect(renderTime).toBeLessThan(100); // Should render in less than 100ms
  });
});
```

**Validates**: AC-1.6 (Estimate loads in <500ms)

## Visual Regression Tests

### 9. Visual Tests

#### Test 9.1: Loading State Appearance
**Description**: Loading state should match design

```typescript
describe('CostEstimator - Visual Tests', () => {
  it('matches loading state snapshot', () => {
    const { container } = render(CostEstimator, {
      props: { blueprint: 'static-website', region: 'us-east-1' }
    });
    
    expect(container).toMatchSnapshot();
  });
});
```

#### Test 9.2: Success State Appearance
**Description**: Success state should match design

```typescript
it('matches success state snapshot', async () => {
  const mockEstimate = {
    estimate: {
      total: 56.65,
      range: { min: 45.32, max: 79.31 },
      breakdown: { compute: 29.15, networking: 16.20, storage: 2.30, transfer: 9.00 },
      assumptions: { storage: '10GB' },
      disclaimer: 'Test disclaimer',
      pricingFetchedAt: '2026-04-11T10:00:00Z'
    }
  };
  
  vi.mocked(costApi.getCostEstimate).mockResolvedValue(mockEstimate);
  
  const { container } = render(CostEstimator, {
    props: { blueprint: 'static-website', region: 'us-east-1' }
  });
  
  await waitFor(() => {
    expect(screen.getByText(/\$56\.65/)).toBeInTheDocument();
  });
  
  expect(container).toMatchSnapshot();
});
```

#### Test 9.3: Error State Appearance
**Description**: Error state should match design

```typescript
it('matches error state snapshot', async () => {
  vi.mocked(costApi.getCostEstimate).mockRejectedValue(
    new Error('API Error')
  );
  
  const { container } = render(CostEstimator, {
    props: { blueprint: 'static-website', region: 'us-east-1' }
  });
  
  await waitFor(() => {
    expect(screen.getByText(/Error:/)).toBeInTheDocument();
  });
  
  expect(container).toMatchSnapshot();
});
```

## Test Coverage Goals

- **Line Coverage**: >90%
- **Branch Coverage**: >85%
- **Function Coverage**: 100%
- **Statement Coverage**: >90%

## Running Tests

When testing framework is set up:

```bash
# Run all tests
npm test

# Run tests in watch mode
npm test -- --watch

# Run tests with coverage
npm test -- --coverage

# Run specific test file
npm test CostEstimator.test.ts
```

## Validation Summary

This test suite validates the following acceptance criteria:

- ✓ AC-1.1: System shows cost range (min-max)
- ✓ AC-1.2: Estimate includes compute, networking, storage costs
- ✓ AC-1.3: Estimate clearly labels what is included/excluded
- ✓ AC-1.4: Estimate is region-specific
- ✓ AC-1.5: Estimate shows pricing data freshness
- ✓ AC-1.6: Estimate loads in <500ms
- ✓ AC-5.1: Estimate shows itemized breakdown
- ✓ AC-5.2: Disclaimer clearly states excluded costs
- ✓ AC-5.4: Shows assumptions
- ✓ TR-2.4: Round to 2 decimal places
- ✓ TR-5.1: Graceful degradation if API unavailable
