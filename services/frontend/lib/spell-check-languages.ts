/**
 * The languages the spell checker offers.
 *
 * From config/settings.defaults.js. A language with a `dic` has a Hunspell
 * dictionary shipped with the client and can be checked; the ones without
 * are offered because a project can still declare them, and are simply not
 * checked -- which is what the original does too.
 */
export type SpellCheckLanguage = {
  code: string;
  name: string;
  dic?: string;
};

export const spellCheckLanguages: SpellCheckLanguage[] = [
  {
    code: 'en',
    name: 'English',
  },
  {
    code: 'en_US',
    name: 'English (American)',
    dic: 'en_US',
  },
  {
    code: 'en_GB',
    name: 'English (British)',
    dic: 'en_GB',
  },
  {
    code: 'en_CA',
    name: 'English (Canadian)',
    dic: 'en_CA',
  },
  {
    code: 'en_AU',
    name: 'English (Australian)',
    dic: 'en_AU',
  },
  {
    code: 'en_ZA',
    name: 'English (South African)',
    dic: 'en_ZA',
  },
  {
    code: 'af',
    name: 'Afrikaans',
    dic: 'af_ZA',
  },
  {
    code: 'an',
    name: 'Aragonese',
    dic: 'an_ES',
  },
  {
    code: 'ar',
    name: 'Arabic',
    dic: 'ar',
  },
  {
    code: 'be_BY',
    name: 'Belarusian',
    dic: 'be_BY',
  },
  {
    code: 'eu',
    name: 'Basque',
    dic: 'eu',
  },
  {
    code: 'bn_BD',
    name: 'Bengali',
    dic: 'bn_BD',
  },
  {
    code: 'bs_BA',
    name: 'Bosnian',
    dic: 'bs_BA',
  },
  {
    code: 'br',
    name: 'Breton',
    dic: 'br_FR',
  },
  {
    code: 'bg',
    name: 'Bulgarian',
    dic: 'bg_BG',
  },
  {
    code: 'ca',
    name: 'Catalan',
    dic: 'ca',
  },
  {
    code: 'hr',
    name: 'Croatian',
    dic: 'hr_HR',
  },
  {
    code: 'cs',
    name: 'Czech',
    dic: 'cs_CZ',
  },
  {
    code: 'da',
    name: 'Danish',
    dic: 'da_DK',
  },
  {
    code: 'nl',
    name: 'Dutch',
    dic: 'nl',
  },
  {
    code: 'dz',
    name: 'Dzongkha',
    dic: 'dz',
  },
  {
    code: 'eo',
    name: 'Esperanto',
    dic: 'eo',
  },
  {
    code: 'et',
    name: 'Estonian',
    dic: 'et_EE',
  },
  {
    code: 'fo',
    name: 'Faroese',
    dic: 'fo',
  },
  {
    code: 'fr',
    name: 'French',
    dic: 'fr',
  },
  {
    code: 'gl',
    name: 'Galician',
    dic: 'gl_ES',
  },
  {
    code: 'de',
    name: 'German',
    dic: 'de_DE',
  },
  {
    code: 'de_AT',
    name: 'German (Austria)',
    dic: 'de_AT',
  },
  {
    code: 'de_CH',
    name: 'German (Switzerland)',
    dic: 'de_CH',
  },
  {
    code: 'el',
    name: 'Greek',
    dic: 'el_GR',
  },
  {
    code: 'gug_PY',
    name: 'Guarani',
    dic: 'gug_PY',
  },
  {
    code: 'gu_IN',
    name: 'Gujarati',
    dic: 'gu_IN',
  },
  {
    code: 'he_IL',
    name: 'Hebrew',
    dic: 'he_IL',
  },
  {
    code: 'hi_IN',
    name: 'Hindi',
    dic: 'hi_IN',
  },
  {
    code: 'hu_HU',
    name: 'Hungarian',
    dic: 'hu_HU',
  },
  {
    code: 'is_IS',
    name: 'Icelandic',
    dic: 'is_IS',
  },
  {
    code: 'id',
    name: 'Indonesian',
    dic: 'id_ID',
  },
  {
    code: 'ga',
    name: 'Irish',
    dic: 'ga_IE',
  },
  {
    code: 'it',
    name: 'Italian',
    dic: 'it_IT',
  },
  {
    code: 'kk',
    name: 'Kazakh',
    dic: 'kk_KZ',
  },
  {
    code: 'ko',
    name: 'Korean',
    dic: 'ko',
  },
  {
    code: 'ku',
    name: 'Kurdish',
  },
  {
    code: 'kmr',
    name: 'Kurmanji',
    dic: 'kmr_Latn',
  },
  {
    code: 'lv',
    name: 'Latvian',
    dic: 'lv_LV',
  },
  {
    code: 'lt',
    name: 'Lithuanian',
    dic: 'lt_LT',
  },
  {
    code: 'lo_LA',
    name: 'Laotian',
    dic: 'lo_LA',
  },
  {
    code: 'ml_IN',
    name: 'Malayalam',
    dic: 'ml_IN',
  },
  {
    code: 'mn_MN',
    name: 'Mongolian',
    dic: 'mn_MN',
  },
  {
    code: 'nr',
    name: 'Ndebele',
  },
  {
    code: 'ne_NP',
    name: 'Nepali',
    dic: 'ne_NP',
  },
  {
    code: 'ns',
    name: 'Northern Sotho',
  },
  {
    code: 'no',
    name: 'Norwegian',
  },
  {
    code: 'nb_NO',
    name: 'Norwegian (Bokmål)',
    dic: 'nb_NO',
  },
  {
    code: 'nn_NO',
    name: 'Norwegian (Nynorsk)',
    dic: 'nn_NO',
  },
  {
    code: 'oc_FR',
    name: 'Occitan',
    dic: 'oc_FR',
  },
  {
    code: 'fa',
    name: 'Persian',
    dic: 'fa_IR',
  },
  {
    code: 'pl',
    name: 'Polish',
    dic: 'pl_PL',
  },
  {
    code: 'pt_BR',
    name: 'Portuguese (Brazilian)',
    dic: 'pt_BR',
  },
  {
    code: 'pt_PT',
    name: 'Portuguese (European)',
    dic: 'pt_PT',
  },
  {
    code: 'pa',
    name: 'Punjabi',
  },
  {
    code: 'ro',
    name: 'Romanian',
    dic: 'ro_RO',
  },
  {
    code: 'ru',
    name: 'Russian',
    dic: 'ru_RU',
  },
  {
    code: 'gd_GB',
    name: 'Scottish Gaelic',
    dic: 'gd_GB',
  },
  {
    code: 'sr_RS',
    name: 'Serbian',
    dic: 'sr_RS',
  },
  {
    code: 'si_LK',
    name: 'Sinhala',
    dic: 'si_LK',
  },
  {
    code: 'sk',
    name: 'Slovak',
    dic: 'sk_SK',
  },
  {
    code: 'sl',
    name: 'Slovenian',
    dic: 'sl_SI',
  },
  {
    code: 'st',
    name: 'Southern Sotho',
  },
  {
    code: 'es',
    name: 'Spanish',
    dic: 'es_ES',
  },
  {
    code: 'sw_TZ',
    name: 'Swahili',
    dic: 'sw_TZ',
  },
  {
    code: 'sv',
    name: 'Swedish',
    dic: 'sv_SE',
  },
  {
    code: 'tl',
    name: 'Tagalog',
    dic: 'tl',
  },
  {
    code: 'te_IN',
    name: 'Telugu',
    dic: 'te_IN',
  },
  {
    code: 'th_TH',
    name: 'Thai',
    dic: 'th_TH',
  },
  {
    code: 'bo',
    name: 'Tibetan',
    dic: 'bo',
  },
  {
    code: 'ts',
    name: 'Tsonga',
  },
  {
    code: 'tn',
    name: 'Tswana',
  },
  {
    code: 'tr_TR',
    name: 'Turkish',
    dic: 'tr_TR',
  },
  {
    code: 'uk_UA',
    name: 'Ukrainian',
    dic: 'uk_UA',
  },
  {
    code: 'hsb',
    name: 'Upper Sorbian',
  },
  {
    code: 'uz_UZ',
    name: 'Uzbek',
    dic: 'uz_UZ',
  },
  {
    code: 'vi_VN',
    name: 'Vietnamese',
    dic: 'vi_VN',
  },
  {
    code: 'cy',
    name: 'Welsh',
  },
  {
    code: 'xh',
    name: 'Xhosa',
  },
];
