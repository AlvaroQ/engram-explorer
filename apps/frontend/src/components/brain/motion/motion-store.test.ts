import { describe, it, expect, beforeEach } from 'vitest'
import { motionStore } from './motion-store'
import type { TweenHandle } from './interpolator'

// ---------------------------------------------------------------------------
// C1.5 — motion-store lifecycle
// ---------------------------------------------------------------------------

describe('motionStore', () => {
  beforeEach(() => {
    // Reset state before each test
    motionStore.cancelAll()
  })

  it('registry starts empty — hasActive() is false', () => {
    expect(motionStore.hasActive()).toBe(false)
  })

  it('after register(handle), hasActive() is true', () => {
    const fakeHandle: TweenHandle = {
      cancel: () => {},
      step: () => {},
      get done() { return false },
    }
    motionStore.register(fakeHandle)
    expect(motionStore.hasActive()).toBe(true)
  })

  it('tick(deltaMs) calls step(deltaMs) on each active handle', () => {
    const stepsReceived: number[] = []
    let isDone = false
    const fakeHandle: TweenHandle = {
      cancel: () => {},
      step: (deltaMs) => { stepsReceived.push(deltaMs) },
      get done() { return isDone },
    }
    motionStore.register(fakeHandle)

    motionStore.tick(16)
    expect(stepsReceived).toEqual([16])

    motionStore.tick(32)
    expect(stepsReceived).toEqual([16, 32])

    isDone = true
    motionStore.tick(16)
    // step is NOT called on done handles (swept before or after)
    expect(stepsReceived.length).toBe(2)
  })

  it('after tween completes (done=true), tick removes it and hasActive() is false', () => {
    let isDone = false
    const fakeHandle: TweenHandle = {
      cancel: () => { isDone = true },
      step: () => {},
      get done() { return isDone },
    }
    motionStore.register(fakeHandle)
    expect(motionStore.hasActive()).toBe(true)

    // Simulate the tween completing externally (step sets done)
    isDone = true
    motionStore.tick(16)

    expect(motionStore.hasActive()).toBe(false)
  })

  it('cancelAll() removes all handles and hasActive() returns false', () => {
    const fakeHandle1: TweenHandle = {
      cancel: () => {},
      step: () => {},
      get done() { return false },
    }
    const fakeHandle2: TweenHandle = {
      cancel: () => {},
      step: () => {},
      get done() { return false },
    }
    motionStore.register(fakeHandle1)
    motionStore.register(fakeHandle2)
    expect(motionStore.hasActive()).toBe(true)

    motionStore.cancelAll()
    expect(motionStore.hasActive()).toBe(false)
  })

  it('tick(deltaMs) does not call cancel on completed handles — only sweeps them', () => {
    let cancelCalled = false
    let isDone = false
    const fakeHandle: TweenHandle = {
      cancel: () => { cancelCalled = true },
      step: () => {},
      get done() { return isDone },
    }
    motionStore.register(fakeHandle)

    // Not done yet — cancel should NOT be called just from tick
    motionStore.tick(16)
    expect(cancelCalled).toBe(false) // still active

    isDone = true
    motionStore.tick(16)
    // After done, the handle should be swept from the registry — no cancel call
    expect(cancelCalled).toBe(false)
    expect(motionStore.hasActive()).toBe(false)
  })

  it('tick calls step then sweeps: step on a handle that becomes done mid-tick is allowed', () => {
    let isDone = false
    const stepDeltas: number[] = []
    const fakeHandle: TweenHandle = {
      cancel: () => {},
      step: (deltaMs) => {
        stepDeltas.push(deltaMs)
        isDone = true // mark done inside step (natural completion)
      },
      get done() { return isDone },
    }
    motionStore.register(fakeHandle)

    motionStore.tick(16)
    // step was called once, then handle is swept
    expect(stepDeltas).toEqual([16])
    expect(motionStore.hasActive()).toBe(false)
  })
})
