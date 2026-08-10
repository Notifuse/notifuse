import { describe, it, expect } from 'vitest';
import { parseCustomDimensions } from './custom-dimensions';

describe('parseCustomDimensions', () => {
  describe('basic parsing', () => {
    it('parses single dimension (custom_1)', () => {
      const result = parseCustomDimensions('https://example.com?custom_1=campaign_a');
      expect(result).toEqual({ 1: 'campaign_a' });
    });

    it('parses multiple dimensions', () => {
      const result = parseCustomDimensions('https://example.com?custom_1=first&custom_5=fifth&custom_10=tenth');
      expect(result).toEqual({
        1: 'first',
        5: 'fifth',
        10: 'tenth',
      });
    });

    it('handles all indices 1-10', () => {
      const url = 'https://example.com?' +
        Array.from({ length: 10 }, (_, i) => `custom_${i + 1}=val${i + 1}`).join('&');
      const result = parseCustomDimensions(url);

      expect(Object.keys(result)).toHaveLength(10);
      for (let i = 1; i <= 10; i++) {
        expect(result[i]).toBe(`val${i}`);
      }
    });

    it('returns empty object for URL without custom params', () => {
      const result = parseCustomDimensions('https://example.com?utm_source=google');
      expect(result).toEqual({});
    });
  });

  describe('index validation', () => {
    it('ignores custom_0 (out of range)', () => {
      const result = parseCustomDimensions('https://example.com?custom_0=invalid&custom_1=valid');
      expect(result).toEqual({ 1: 'valid' });
      expect(result[0]).toBeUndefined();
    });

    it('ignores custom_11 (out of range)', () => {
      const result = parseCustomDimensions('https://example.com?custom_11=invalid&custom_10=valid');
      expect(result).toEqual({ 10: 'valid' });
      expect(result[11]).toBeUndefined();
    });

    it('ignores negative indices', () => {
      const result = parseCustomDimensions('https://example.com?custom_-1=invalid&custom_1=valid');
      expect(result).toEqual({ 1: 'valid' });
    });
  });

  describe('value validation', () => {
    it('ignores values > 256 characters', () => {
      const longValue = 'a'.repeat(257);
      const result = parseCustomDimensions(`https://example.com?custom_1=${longValue}&custom_2=valid`);
      expect(result).toEqual({ 2: 'valid' });
      expect(result[1]).toBeUndefined();
    });

    it('accepts values exactly 256 characters', () => {
      const exactValue = 'a'.repeat(256);
      const result = parseCustomDimensions(`https://example.com?custom_1=${exactValue}`);
      expect(result).toEqual({ 1: exactValue });
    });

    it('handles empty string values', () => {
      const result = parseCustomDimensions('https://example.com?custom_1=');
      expect(result).toEqual({ 1: '' });
    });
  });

  describe('URL encoding', () => {
    it('handles URL-encoded values', () => {
      const result = parseCustomDimensions('https://example.com?custom_1=hello%20world');
      expect(result).toEqual({ 1: 'hello world' });
    });

    it('handles special characters', () => {
      const result = parseCustomDimensions('https://example.com?custom_1=a%2Bb%3Dc');
      expect(result).toEqual({ 1: 'a+b=c' });
    });

    it('handles unicode characters', () => {
      const result = parseCustomDimensions('https://example.com?custom_1=%E4%B8%AD%E6%96%87');
      expect(result).toEqual({ 1: '中文' });
    });
  });

  describe('error handling', () => {
    it('handles invalid URLs gracefully', () => {
      const result = parseCustomDimensions('not-a-valid-url');
      expect(result).toEqual({});
    });

    it('handles empty string', () => {
      const result = parseCustomDimensions('');
      expect(result).toEqual({});
    });

    it('handles URL without query string', () => {
      const result = parseCustomDimensions('https://example.com/page');
      expect(result).toEqual({});
    });
  });

  describe('mixed parameters', () => {
    it('ignores non-custom parameters', () => {
      const result = parseCustomDimensions(
        'https://example.com?utm_source=google&custom_1=dim1&fbclid=abc&custom_2=dim2'
      );
      expect(result).toEqual({ 1: 'dim1', 2: 'dim2' });
    });

    it('handles duplicate custom params (first value wins)', () => {
      const result = parseCustomDimensions('https://example.com?custom_1=first&custom_1=second');
      expect(result).toEqual({ 1: 'first' });
    });
  });
});
