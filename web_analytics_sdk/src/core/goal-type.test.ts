import { describe, it, expect, beforeEach, vi } from 'vitest';
import { SessionState, type SessionStateConfig } from './session-state';
import { VALID_GOAL_TYPES, type GoalType } from '../types';

/**
 * The goal type is required in the SDK and lenient on the wire.
 *
 * Requiring it here is the whole point: only the site knows whether a conversion
 * is a purchase or a lead, and an untyped goal cannot be matched by any of the
 * goal-based segment conditions. Guessing server-side would corrupt revenue
 * reporting, so the call site has to say.
 *
 * The server does NOT mirror this strictness — it normalises anything it does not
 * recognise to 'other' and still records the conversion. See
 * internal/domain/web_analytics_goal_type_test.go for why.
 */
describe('goal type', () => {
  let sessionState: SessionState;

  const config: SessionStateConfig = {
    workspace_id: 'test-ws',
    session_id: 'sess-123',
    created_at: Date.now() - 10000,
  };

  beforeEach(() => {
    sessionState = new SessionState(config);
    sessionState.addPageview('/checkout');
  });

  describe('addGoal', () => {
    it('carries the declared type onto the action as goal_type', () => {
      sessionState.addGoal('purchase', 'purchase', 49.9);

      const actions = sessionState.getActions();
      const goal = actions.find((a) => a.type === 'goal');
      expect(goal).toBeDefined();
      expect(goal).toMatchObject({ name: 'purchase', goal_type: 'purchase' });
    });

    it('keeps goal_type distinct from the action discriminator', () => {
      sessionState.addGoal('demo_request', 'lead');

      const goal = sessionState.getActions().find((a) => a.type === 'goal');
      // `type` says which KIND of action this is; goal_type says what the goal is.
      // Collapsing the two would make every goal look like a pageview to the server.
      expect(goal).toMatchObject({ type: 'goal', goal_type: 'lead' });
    });

    it.each(VALID_GOAL_TYPES)('accepts %s', (goalType) => {
      sessionState.addGoal(`goal_${goalType}`, goalType);

      const goal = sessionState
        .getActions()
        .find((a) => a.type === 'goal' && a.name === `goal_${goalType}`);
      expect(goal).toMatchObject({ goal_type: goalType });
    });

    it('types a goal that carries no value', () => {
      // A lead or a signup usually has no money attached. Typing only the ones
      // that do would leave the wrong half of the funnel unsegmentable.
      sessionState.addGoal('newsletter_signup', 'signup');

      const goal = sessionState
        .getActions()
        .find((a) => a.type === 'goal' && a.name === 'newsletter_signup');
      expect(goal).toMatchObject({ goal_type: 'signup' });
      expect(goal && 'value' in goal).toBe(false);
    });

    it('gives each goal its own type within one session', () => {
      sessionState.addGoal('add_to_cart', 'other');
      sessionState.addGoal('purchase', 'purchase', 49.9);

      const goals = sessionState.getActions().filter((a) => a.type === 'goal');
      expect(goals.map((g) => [g.name, (g as { goal_type: GoalType }).goal_type])).toEqual([
        ['add_to_cart', 'other'],
        ['purchase', 'purchase'],
      ]);
    });
  });

  describe('VALID_GOAL_TYPES', () => {
    it('matches domain.ValidGoalTypes on the server, in order', () => {
      // Drift here is silent: a type this SDK allows but the server does not
      // recognise is quietly recorded as 'other'.
      expect([...VALID_GOAL_TYPES]).toEqual([
        'purchase',
        'subscription',
        'lead',
        'signup',
        'booking',
        'trial',
        'other',
      ]);
    });
  });
});

/**
 * trackGoal's own validation lives on the SDK class, which needs a configured
 * instance. These cover the rejection contract without a full browser harness.
 */
describe('trackGoal type validation', () => {
  beforeEach(() => {
    vi.resetModules();
    localStorage.clear();
    sessionStorage.clear();
  });

  const configure = async () => {
    const { NotifuseAnalyticsSDK } = await import('../sdk');
    const sdk = new NotifuseAnalyticsSDK();
    await sdk.init({
      workspace_id: 'ws_test',
      endpoint: 'https://api.example.com',
    });
    return sdk;
  };

  it('rejects a goal with no type', async () => {
    const sdk = await configure();
    await expect(
      // The cast is the point: JavaScript callers have no compiler to stop them,
      // so the runtime check has to exist as well as the type.
      sdk.trackGoal({ action: 'purchase' } as unknown as { action: string; type: GoalType }),
    ).rejects.toThrow(/trackGoal requires a type/);
  });

  it('rejects a type the server would not recognise', async () => {
    const sdk = await configure();
    await expect(
      sdk.trackGoal({ action: 'purchase', type: 'nonsense' as GoalType }),
    ).rejects.toThrow(/trackGoal requires a type/);
  });

  it('names the accepted types in the error, so the fix is obvious', async () => {
    const sdk = await configure();
    await expect(
      sdk.trackGoal({ action: 'purchase', type: '' as GoalType }),
    ).rejects.toThrow(/purchase, subscription, lead, signup, booking, trial, other/);
  });

  it('accepts a valid type', async () => {
    const sdk = await configure();
    await expect(
      sdk.trackGoal({ action: 'purchase', type: 'purchase', value: 49.9 }),
    ).resolves.not.toThrow();
  });
});
