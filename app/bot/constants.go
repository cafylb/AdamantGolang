package bot

const (
	fifty = 0.89
	hundred = 1.75
	fifteen = 0.45
	twentyfive = 0.70
)

type gift struct {
	Price   float64
	Icon    string
	EmojiID int64
	GiftID  int64
}

var Gifts = []gift{
	{Price: fifty, Icon: "🎁", EmojiID: 5447213743417105726, GiftID: 6026193266406327981},
	{Price: fifty, Icon: "🎁", EmojiID: 5393309541620291208, GiftID: 5969796561943660080},
	{Price: fifty, Icon: "🎁", EmojiID: 5359736160224586485, GiftID: 5935895822435615975},
	{Price: fifty, Icon: "🎁", EmojiID: 5317000922096769303, GiftID: 5893356958802511476},
	{Price: fifty, Icon: "🧸", EmojiID: 5289761157173775507, GiftID: 5866352046986232958},
	{Price: fifty, Icon: "🎁", EmojiID: 5226661632259691727, GiftID: 5800655655995968830},
	{Price: fifty, Icon: "🎁", EmojiID: 5224628072619216265, GiftID: 5801108895304779062},
	{Price: fifty, Icon: "🎁", EmojiID: 5379850840691476775, GiftID: 5956217000635139069},
	{Price: fifty, Icon: "🎄", EmojiID: 5345935030143196497, GiftID: 5922558454332916696},
	{Price: hundred, Icon: "💎", EmojiID: 5280922999241859582, GiftID: 5170521118301225164},
	{Price: hundred, Icon: "💍", EmojiID: 5280651583078556009, GiftID: 5170521118301225164},
	{Price: hundred, Icon: "🏆", EmojiID: 5280769763398671636, GiftID: 5168043875654172773},
	{Price: fifty, Icon: "💐", EmojiID: 5280774333243873175, GiftID: 5170314324215857265},
	{Price: fifty, Icon: "🎂", EmojiID: 5280659198055572187, GiftID: 5170144170496491616},
	{Price: fifty, Icon: "🍾", EmojiID: 5451905784734574339, GiftID: 6028601630662853006},
	{Price: fifty, Icon: "🚀", EmojiID: 5283080528818360566, GiftID: 5170564780938756245},
	{Price: twentyfive, Icon: "🎁", EmojiID: 5280615440928758599, GiftID: 5170250947678437525},
	{Price: twentyfive, Icon: "🌹", EmojiID: 5280947338821524402, GiftID: 5168103777563050263},
	{Price: fifteen, Icon: "🧸", EmojiID: 5280598054901145762, GiftID: 5170233102089322756},
	{Price: fifteen, Icon: "💝", EmojiID: 5283228279988309088, GiftID: 5170145012310081615},
}