import { describe, it, expect } from 'vitest';
import { parseUTMParams, hasUTMParams, parseReferrer, DEFAULT_AD_CLICK_IDS } from './utm';

describe('UTM Parsing', () => {
  describe('parseUTMParams', () => {
    describe('standard UTM parameters', () => {
      it('parses utm_source from URL', () => {
        const result = parseUTMParams('https://example.com?utm_source=google');
        expect(result.source).toBe('google');
      });

      it('parses utm_medium from URL', () => {
        const result = parseUTMParams('https://example.com?utm_medium=cpc');
        expect(result.medium).toBe('cpc');
      });

      it('parses utm_campaign from URL', () => {
        const result = parseUTMParams('https://example.com?utm_campaign=summer_sale');
        expect(result.campaign).toBe('summer_sale');
      });

      it('parses utm_term from URL', () => {
        const result = parseUTMParams('https://example.com?utm_term=running+shoes');
        expect(result.term).toBe('running shoes');
      });

      it('parses utm_content from URL', () => {
        const result = parseUTMParams('https://example.com?utm_content=banner_ad');
        expect(result.content).toBe('banner_ad');
      });

      it('parses all UTM parameters together', () => {
        const url =
          'https://example.com?utm_source=google&utm_medium=cpc&utm_campaign=spring&utm_term=shoes&utm_content=header';
        const result = parseUTMParams(url);
        expect(result).toEqual({
          source: 'google',
          medium: 'cpc',
          campaign: 'spring',
          term: 'shoes',
          content: 'header',
          id: null,
          id_from: null,
        });
      });

      it('returns null for missing parameters', () => {
        const result = parseUTMParams('https://example.com');
        expect(result).toEqual({
          source: null,
          medium: null,
          campaign: null,
          term: null,
          content: null,
          id: null,
          id_from: null,
        });
      });
    });

    describe('ad click ID detection', () => {
      it('detects gclid (Google Ads)', () => {
        const result = parseUTMParams('https://example.com?gclid=abc123');
        expect(result.id).toBe('abc123');
        expect(result.id_from).toBe('gclid');
      });

      it('detects fbclid (Facebook/Meta)', () => {
        const result = parseUTMParams('https://example.com?fbclid=fb_xyz');
        expect(result.id).toBe('fb_xyz');
        expect(result.id_from).toBe('fbclid');
      });

      it('detects msclkid (Microsoft)', () => {
        const result = parseUTMParams('https://example.com?msclkid=ms_123');
        expect(result.id).toBe('ms_123');
        expect(result.id_from).toBe('msclkid');
      });

      it('detects dclid (DoubleClick)', () => {
        const result = parseUTMParams('https://example.com?dclid=dc_456');
        expect(result.id).toBe('dc_456');
        expect(result.id_from).toBe('dclid');
      });

      it('detects twclid (Twitter/X)', () => {
        const result = parseUTMParams('https://example.com?twclid=tw_789');
        expect(result.id).toBe('tw_789');
        expect(result.id_from).toBe('twclid');
      });

      it('detects ttclid (TikTok)', () => {
        const result = parseUTMParams('https://example.com?ttclid=tt_abc');
        expect(result.id).toBe('tt_abc');
        expect(result.id_from).toBe('ttclid');
      });

      it('detects li_fat_id (LinkedIn)', () => {
        const result = parseUTMParams('https://example.com?li_fat_id=li_def');
        expect(result.id).toBe('li_def');
        expect(result.id_from).toBe('li_fat_id');
      });

      it('detects wbraid (Google iOS)', () => {
        const result = parseUTMParams('https://example.com?wbraid=wb_ghi');
        expect(result.id).toBe('wb_ghi');
        expect(result.id_from).toBe('wbraid');
      });

      it('detects gbraid (Google cross-device)', () => {
        const result = parseUTMParams('https://example.com?gbraid=gb_jkl');
        expect(result.id).toBe('gb_jkl');
        expect(result.id_from).toBe('gbraid');
      });

      it('first match wins (priority order)', () => {
        // gclid comes before fbclid in DEFAULT_AD_CLICK_IDS
        const result = parseUTMParams('https://example.com?fbclid=fb_123&gclid=gc_456');
        expect(result.id).toBe('gc_456');
        expect(result.id_from).toBe('gclid');
      });

      it('uses custom adClickIds array when provided', () => {
        const customIds = ['custom_id', 'another_id'];
        const result = parseUTMParams('https://example.com?custom_id=custom_value', customIds);
        expect(result.id).toBe('custom_value');
        expect(result.id_from).toBe('custom_id');
      });

      it('ignores default IDs when custom array provided', () => {
        const customIds = ['custom_id'];
        const result = parseUTMParams('https://example.com?gclid=gc_123', customIds);
        expect(result.id).toBeNull();
        expect(result.id_from).toBeNull();
      });
    });
  });

  describe('DEFAULT_AD_CLICK_IDS', () => {
    it('contains all 9 expected ad click IDs', () => {
      expect(DEFAULT_AD_CLICK_IDS).toEqual([
        'gclid',
        'fbclid',
        'msclkid',
        'dclid',
        'twclid',
        'ttclid',
        'li_fat_id',
        'wbraid',
        'gbraid',
      ]);
    });
  });

  describe('hasUTMParams', () => {
    it('returns true when source is present', () => {
      expect(hasUTMParams({ source: 'google', medium: null, campaign: null, term: null, content: null, id: null, id_from: null })).toBe(true);
    });

    it('returns true when medium is present', () => {
      expect(hasUTMParams({ source: null, medium: 'cpc', campaign: null, term: null, content: null, id: null, id_from: null })).toBe(true);
    });

    it('returns true when campaign is present', () => {
      expect(hasUTMParams({ source: null, medium: null, campaign: 'spring', term: null, content: null, id: null, id_from: null })).toBe(true);
    });

    it('returns true when term is present', () => {
      expect(hasUTMParams({ source: null, medium: null, campaign: null, term: 'shoes', content: null, id: null, id_from: null })).toBe(true);
    });

    it('returns true when content is present', () => {
      expect(hasUTMParams({ source: null, medium: null, campaign: null, term: null, content: 'banner', id: null, id_from: null })).toBe(true);
    });

    it('returns true when id is present', () => {
      expect(hasUTMParams({ source: null, medium: null, campaign: null, term: null, content: null, id: 'abc123', id_from: 'gclid' })).toBe(true);
    });

    it('returns false when all values are null', () => {
      expect(hasUTMParams({ source: null, medium: null, campaign: null, term: null, content: null, id: null, id_from: null })).toBe(false);
    });
  });

  describe('parseReferrer', () => {
    it('extracts hostname as domain', () => {
      const result = parseReferrer('https://www.google.com/search?q=test');
      expect(result.domain).toBe('www.google.com');
    });

    it('extracts pathname as path', () => {
      const result = parseReferrer('https://www.google.com/search?q=test');
      expect(result.path).toBe('/search');
    });

    it('handles complex paths', () => {
      const result = parseReferrer('https://example.com/blog/article/2024/my-post');
      expect(result.domain).toBe('example.com');
      expect(result.path).toBe('/blog/article/2024/my-post');
    });

    it('returns { domain: null, path: null } for empty string', () => {
      const result = parseReferrer('');
      expect(result).toEqual({ domain: null, path: null });
    });

    it('returns { domain: null, path: null } for invalid URL', () => {
      const result = parseReferrer('not a valid url');
      expect(result).toEqual({ domain: null, path: null });
    });

    it('handles root path correctly', () => {
      const result = parseReferrer('https://example.com');
      expect(result.domain).toBe('example.com');
      expect(result.path).toBe('/');
    });

    it('handles URLs with ports', () => {
      const result = parseReferrer('https://example.com:8080/page');
      // hostname doesn't include port in URL API
      expect(result.domain).toBe('example.com');
      expect(result.path).toBe('/page');
    });
  });
});
