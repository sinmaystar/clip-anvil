package contextcompact

type Config struct {
	Enabled                     bool `mapstructure:"enabled"`
	EnabledSet                  bool `mapstructure:"-"`
	ModelContextWindowTokens    int  `mapstructure:"model_context_window_tokens"`
	MicroTriggerTokens          int  `mapstructure:"micro_trigger_tokens"`
	MicroTargetTokens           int  `mapstructure:"micro_target_tokens"`
	MicroMinReductionTokens     int  `mapstructure:"micro_min_reduction_tokens"`
	FullTriggerTokens           int  `mapstructure:"full_trigger_tokens"`
	FullTargetTokens            int  `mapstructure:"full_target_tokens"`
	PreserveRecentUserMessages  int  `mapstructure:"preserve_recent_user_messages"`
	PreserveRecentTotalMessages int  `mapstructure:"preserve_recent_total_messages"`
	SearchMaxResults            int  `mapstructure:"search_max_results"`
	MediaImageInputTokenWeight  int  `mapstructure:"media_image_input_token_weight"`
	MediaCardMaxChars           int  `mapstructure:"media_card_max_chars"`
}

func DefaultConfig() Config {
	return Config{
		Enabled:                     true,
		ModelContextWindowTokens:    256000,
		MicroTriggerTokens:          180000,
		MicroTargetTokens:           150000,
		MicroMinReductionTokens:     8000,
		FullTriggerTokens:           200000,
		FullTargetTokens:            140000,
		PreserveRecentUserMessages:  6,
		PreserveRecentTotalMessages: 40,
		SearchMaxResults:            50,
		MediaImageInputTokenWeight:  1500,
		MediaCardMaxChars:           1200,
	}
}

func (c Config) WithDefaults() Config {
	defaults := DefaultConfig()
	if !c.EnabledSet && !c.Enabled {
		c.Enabled = defaults.Enabled
	}
	if c.ModelContextWindowTokens <= 0 {
		c.ModelContextWindowTokens = defaults.ModelContextWindowTokens
	}
	if c.MicroTriggerTokens <= 0 {
		c.MicroTriggerTokens = defaults.MicroTriggerTokens
	}
	if c.MicroTargetTokens <= 0 {
		c.MicroTargetTokens = defaults.MicroTargetTokens
	}
	if c.MicroMinReductionTokens <= 0 {
		c.MicroMinReductionTokens = defaults.MicroMinReductionTokens
	}
	if c.FullTriggerTokens <= 0 {
		c.FullTriggerTokens = defaults.FullTriggerTokens
	}
	if c.FullTargetTokens <= 0 {
		c.FullTargetTokens = defaults.FullTargetTokens
	}
	if c.PreserveRecentUserMessages <= 0 {
		c.PreserveRecentUserMessages = defaults.PreserveRecentUserMessages
	}
	if c.PreserveRecentTotalMessages <= 0 {
		c.PreserveRecentTotalMessages = defaults.PreserveRecentTotalMessages
	}
	if c.SearchMaxResults <= 0 {
		c.SearchMaxResults = defaults.SearchMaxResults
	}
	if c.MediaImageInputTokenWeight <= 0 {
		c.MediaImageInputTokenWeight = defaults.MediaImageInputTokenWeight
	}
	if c.MediaCardMaxChars <= 0 {
		c.MediaCardMaxChars = defaults.MediaCardMaxChars
	}
	return c
}
