import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  PAYMENT_METHOD_TEMPLATES,
  PAYMENT_METHOD_TYPE_OPTIONS,
} from './payment-method-options'

describe('payment method management presets', () => {
  test('allows administrators to choose Linux DO Credit when adding a method', () => {
    assert.ok(
      PAYMENT_METHOD_TYPE_OPTIONS.some(
        (option) =>
          option.value === 'linuxdo_credit' &&
          option.label === 'Linux DO Credit'
      )
    )
  })

  test('provides a Linux DO Credit quick insert template', () => {
    const template = PAYMENT_METHOD_TEMPLATES.find(
      (item) => item.template.type === 'linuxdo_credit'
    )

    assert.deepEqual(template, {
      name: 'Linux DO Credit',
      template: {
        color: '#111827',
        min_topup: '1',
        name: 'Linux DO Credit',
        type: 'linuxdo_credit',
      },
    })
  })
})
