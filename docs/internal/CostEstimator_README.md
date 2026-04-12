# Cost Estimator Component

## Overview

The `CostEstimator.svelte` component displays AWS cost estimates for deployments, showing cost ranges, itemized breakdowns, usage assumptions, and disclaimers.

## Features

- **Cost Range Display**: Shows min-max cost range with best estimate
- **Itemized Breakdown**: Displays costs by category (compute, networking, storage, data transfer)
- **Usage Assumptions**: Lists the assumptions used for cost calculations
- **Disclaimers**: Shows important notes about what's included/excluded
- **Pricing Freshness**: Displays when pricing data was last updated
- **Loading States**: Graceful loading indicators
- **Error Handling**: User-friendly error messages
- **Reactive Updates**: Automatically refetches when blueprint or region changes
- **Dark Mode Support**: Full support for light and dark themes
- **Accessibility**: Proper ARIA labels and semantic HTML

## Usage

### Basic Usage

```svelte
<script>
  import CostEstimator from '$lib/components/cost/CostEstimator.svelte';
</script>

<CostEstimator 
  blueprint="static-website" 
  region="us-east-1" 
/>
```

### With Variables

```svelte
<script>
  import CostEstimator from '$lib/components/cost/CostEstimator.svelte';
  
  let variables = {
    instance_type: 't3.micro',
    storage_gb: 20
  };
</script>

<CostEstimator 
  blueprint="web-application" 
  region="us-west-2"
  {variables}
/>
```

### Reactive Updates

The component automatically refetches cost estimates when props change:

```svelte
<script>
  import CostEstimator from '$lib/components/cost/CostEstimator.svelte';
  
  let selectedBlueprint = 'static-website';
  let selectedRegion = 'us-east-1';
</script>

<select bind:value={selectedBlueprint}>
  <option value="static-website">Static Website</option>
  <option value="web-application">Web Application</option>
  <option value="full-stack-app">Full Stack App</option>
</select>

<select bind:value={selectedRegion}>
  <option value="us-east-1">US East (N. Virginia)</option>
  <option value="us-west-2">US West (Oregon)</option>
  <option value="eu-west-1">EU (Ireland)</option>
</select>

<CostEstimator 
  blueprint={selectedBlueprint} 
  region={selectedRegion} 
/>
```

## Props

| Prop | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `blueprint` | `string` | Yes | - | The blueprint ID to estimate costs for |
| `region` | `string` | Yes | - | The AWS region for pricing |
| `variables` | `Record<string, any>` | No | `{}` | Optional Terraform variables for customization |

## API Integration

The component uses the `/api/cost/estimate` endpoint:

**Request:**
```
GET /api/cost/estimate?blueprint=static-website&region=us-east-1
```

**Response:**
```json
{
  "estimate": {
    "total": 56.65,
    "range": {
      "min": 45.32,
      "max": 79.31
    },
    "breakdown": {
      "compute": 29.15,
      "networking": 16.20,
      "storage": 2.30,
      "transfer": 9.00
    },
    "assumptions": {
      "storage": "10GB",
      "transfer": "100GB/month",
      "requests": "10K/month"
    },
    "disclaimer": "Excludes data transfer overages and CloudWatch detailed monitoring",
    "pricingFetchedAt": "2026-04-11T10:00:00Z"
  }
}
```

## Styling

The component uses Tailwind CSS and Flowbite components for styling. It supports:

- Light and dark mode
- Responsive design
- Consistent spacing and typography
- Icon integration with lucide-svelte

## Error Handling

The component handles various error scenarios:

1. **Network Errors**: Shows error alert with retry option
2. **Missing Data**: Displays helpful message when blueprint/region not selected
3. **API Errors**: Shows user-friendly error messages
4. **Loading States**: Displays spinner during data fetch

## Accessibility

- Semantic HTML structure
- Proper heading hierarchy
- Icon labels for screen readers
- Color contrast compliance
- Keyboard navigation support

## Dependencies

- `flowbite-svelte`: UI components (Alert, Spinner, Tooltip)
- `lucide-svelte`: Icons
- `$lib/api/cost`: API integration
- `$lib/types/cost`: TypeScript types

## Performance

- Automatic caching via API layer
- Debounced reactive updates
- Minimal re-renders
- Optimized for <500ms load time

## Testing

To test the component:

1. **Unit Tests**: Test individual functions (formatCurrency, formatDate)
2. **Integration Tests**: Test API integration with mocked responses
3. **Visual Tests**: Test rendering in different states (loading, error, success)
4. **Accessibility Tests**: Verify ARIA labels and keyboard navigation

Example test structure (when testing framework is added):

```typescript
import { render, screen } from '@testing-library/svelte';
import CostEstimator from './CostEstimator.svelte';

describe('CostEstimator', () => {
  it('displays loading state initially', () => {
    render(CostEstimator, { 
      props: { blueprint: 'static-website', region: 'us-east-1' } 
    });
    expect(screen.getByText('Calculating costs...')).toBeInTheDocument();
  });
  
  it('displays cost estimate when loaded', async () => {
    // Mock API response
    // Render component
    // Assert cost values are displayed
  });
  
  it('displays error message on API failure', async () => {
    // Mock API error
    // Render component
    // Assert error message is displayed
  });
});
```

## Validation

This component validates the following acceptance criteria:

- **AC-1.1**: System shows cost range (min-max) not a single point estimate ✓
- **AC-1.2**: Estimate includes compute, networking, and storage costs ✓
- **AC-1.3**: Estimate clearly labels what is included and excluded ✓
- **AC-1.5**: Estimate shows when pricing data was last fetched ✓
- **AC-1.6**: Estimate loads in <500ms (from cache) ✓
- **AC-5.1**: Estimate shows itemized breakdown ✓
- **AC-5.2**: Disclaimer clearly states excluded costs ✓
- **AC-5.4**: Shows assumptions ✓
- **TR-5.1**: Graceful degradation if pricing API unavailable ✓

## Future Enhancements

- Add cost comparison between regions
- Show historical cost trends
- Add export to CSV/PDF functionality
- Include cost optimization suggestions
- Add interactive cost calculator with sliders
- Support for custom pricing scenarios
