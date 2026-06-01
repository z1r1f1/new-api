import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { PAYMENT_TYPES } from '../constants'
import type { TopupInfo } from '../types'
import {
  getDefaultPaymentType,
  getMinTopupAmount,
  isLinuxDoCreditPayment,
} from './payment'

function topupInfo(overrides: Partial<TopupInfo> = {}): TopupInfo {
  return {
    enable_online_topup: false,
    enable_stripe_topup: false,
    pay_methods: [],
    min_topup: 1,
    stripe_min_topup: 1,
    amount_options: [],
    discount: {},
    ...overrides,
  }
}

describe('Linux DO Credit payment helpers', () => {
  test('recognizes the dedicated Linux DO Credit payment type', () => {
    assert.equal(isLinuxDoCreditPayment(PAYMENT_TYPES.LINUXDO_CREDIT), true)
    assert.equal(isLinuxDoCreditPayment(PAYMENT_TYPES.ALIPAY), false)
  })

  test('falls back to Linux DO Credit when it is the only enabled gateway', () => {
    const info = topupInfo({
      enable_linuxdo_credit_topup: true,
      linuxdo_credit_min_topup: 5,
    })

    assert.equal(getDefaultPaymentType(info), PAYMENT_TYPES.LINUXDO_CREDIT)
    assert.equal(getMinTopupAmount(info), 5)
  })
})
