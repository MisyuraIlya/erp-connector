package hasavshevet

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"erp-connector/internal/config"
	"erp-connector/internal/erp"
)

type itemState struct {
	ID                      int64
	ItemKey                 string
	VatExampt               bool
	DiscountCode            string
	DiscountPrc             float64
	CurrencyCode            string
	PriceListNumber         *int
	ParentPriceListNumber   *int
	OriginalPrice           float64
	OriginalPriceNoChange   float64
	PriceCalculatedNoChange float64
	Price                   float64
	Discount                float64
	DiscountFound           bool
	IsSpecialPrice          bool
	DiscountPrice           *float64
	LastPrice               *float64
	PriceListGroup          string
	IsFromSpecialTable      bool
	Currency                string
}

type priceListRecord struct {
	ItemKey         string
	Price           float64
	CurrencyCode    string
	PriceListNumber int
}

type discountRecord struct {
	PriceListNumber  int
	ItemDiscountCode string
	DiscountPrc      float64
}

type specialPriceRecord struct {
	ItemKey      string
	Price        float64
	DiscountPrc  float64
	CurrencyCode string
}

type lastPriceRecord struct {
	ItemKey   string
	LastPrice float64
	Currency  string
}

func FetchPriceAndStock(ctx context.Context, dbConn *sql.DB, cfg config.Config, req erp.PriceStockRequest) (erp.PriceStockResult, error) {
	dbNameRaw := strings.TrimSpace(cfg.DB.Database)
	if dbNameRaw == "" {
		return erp.PriceStockResult{}, errors.New("db.database is required")
	}
	dbName := escapeIdentifier(dbNameRaw)
	skus := uniqueStrings(req.SKUList)
	if len(skus) == 0 {
		return erp.PriceStockResult{Items: []erp.PriceStockItem{}}, nil
	}

	parentExtID, err := fetchParentExtID(ctx, dbConn, dbName, req.UserExtID)
	if err != nil {
		return erp.PriceStockResult{}, err
	}

	items, err := fetchBaseItems(ctx, dbConn, dbName, skus)
	if err != nil {
		return erp.PriceStockResult{}, err
	}

	if req.UserExtID != "" {
		userDiscounts, err := fetchDiscounts(ctx, dbConn, dbName, req.UserExtID)
		if err != nil {
			return erp.PriceStockResult{}, err
		}
		userHadAny, err := applyDiscounts(ctx, dbConn, dbName, skus, items, userDiscounts, "1_USER", false)
		if err != nil {
			return erp.PriceStockResult{}, err
		}
		if !userHadAny && parentExtID != "" {
			parentDiscounts, err := fetchDiscounts(ctx, dbConn, dbName, parentExtID)
			if err != nil {
				return erp.PriceStockResult{}, err
			}
			if _, err := applyDiscounts(ctx, dbConn, dbName, skus, items, parentDiscounts, "1_PARENT", true); err != nil {
				return erp.PriceStockResult{}, err
			}
		}

		specialsUser, err := fetchSpecialPrices(ctx, dbConn, dbName, skus, req.UserExtID)
		if err != nil {
			return erp.PriceStockResult{}, err
		}
		var specialsParent map[string]specialPriceRecord
		if parentExtID != "" {
			specialsParent, err = fetchSpecialPrices(ctx, dbConn, dbName, skus, parentExtID)
			if err != nil {
				return erp.PriceStockResult{}, err
			}
		}

		for _, item := range items {
			parentSp, parentOK := specialsParent[item.ItemKey]
			userSp, userOK := specialsUser[item.ItemKey]

			if parentOK || userOK {
				item.IsFromSpecialTable = true
			}

			var chosen specialPriceRecord
			var chosenOK bool
			if parentOK {
				chosen = parentSp
				chosenOK = true
				item.PriceListGroup = "3_SPECIAL_PARENT"
			} else if userOK {
				chosen = userSp
				chosenOK = true
				item.PriceListGroup = "3_SPECIAL_USER"
			}

			if chosenOK {
				spPrice := chosen.Price
				spDisc := chosen.DiscountPrc
				discounted := spPrice - (spPrice*spDisc)/100
				item.OriginalPrice = spPrice
				item.OriginalPriceNoChange = spPrice
				item.PriceCalculatedNoChange = discounted
				item.Price = discounted
				item.Currency = nonEmpty(chosen.CurrencyCode, item.Currency)
				item.IsSpecialPrice = true
				item.DiscountFound = true
				item.DiscountPrice = &discounted
			} else {
				item.IsSpecialPrice = false
				item.DiscountPrice = nil
			}
		}
	}

	for _, item := range items {
		if item.OriginalPrice == 0 && item.Price != 0 {
			item.OriginalPrice = item.Price
			item.OriginalPriceNoChange = item.Price
		}
	}

	lastPrices, err := fetchLastPrices(ctx, dbConn, dbName, skus, req.UserExtID)
	if err != nil {
		return erp.PriceStockResult{}, err
	}

	for _, item := range items {
		if lp, ok := lastPrices[item.ItemKey]; ok {
			item.LastPrice = &lp.LastPrice
			if item.Currency == "" {
				item.Currency = lp.Currency
			}
		}

		if item.PriceListNumber != nil && *item.PriceListNumber == 5 {
			if item.LastPrice != nil {
				lp := *item.LastPrice
				item.OriginalPrice = lp
				item.OriginalPriceNoChange = lp
				item.PriceCalculatedNoChange = lp
				item.Price = lp
				item.PriceListGroup = "PL5_LAST"
				item.Discount = 0
				item.IsSpecialPrice = false
				item.DiscountPrice = nil
			}
		}
	}

	stockByItem, err := fetchStockData(ctx, dbConn, dbName, skus, req.Warehouses)
	if err != nil {
		return erp.PriceStockResult{}, err
	}

	out := make([]erp.PriceStockItem, 0, len(items))
	for _, item := range items {
		priceAfterDiscount := item.PriceCalculatedNoChange
		if item.LastPrice != nil && *item.LastPrice < priceAfterDiscount {
			priceAfterDiscount = *item.LastPrice
		}

		prices := map[string]float64{
			"basePrice":          item.OriginalPrice,
			"priceAfterDiscount": priceAfterDiscount,
		}
		if item.LastPrice != nil {
			prices["lastPrice"] = *item.LastPrice
		}
		if item.DiscountPrice != nil {
			prices["discountPrice"] = *item.DiscountPrice
		}
		if item.Discount != 0 {
			prices["discount"] = item.Discount
		}

		details := map[string]any{
			"currency":              nonEmpty(item.Currency, "N/A"),
			"priceListNumber":       item.PriceListNumber,
			"parentPriceListNumber": item.ParentPriceListNumber,
			"isFromSpecialTable":    item.IsFromSpecialTable,
			"isVatable":             item.VatExampt,
			"priceListGroup":        item.PriceListGroup,
		}

		out = append(out, erp.PriceStockItem{
			SKU:              item.ItemKey,
			Prices:           prices,
			StockByWarehouse: stockByItem[item.ItemKey],
			Details:          details,
		})
	}

	return erp.PriceStockResult{Items: out}, nil
}

func fetchParentExtID(ctx context.Context, dbConn *sql.DB, dbName, userExtID string) (string, error) {
	if userExtID == "" {
		return "", nil
	}
	query := fmt.Sprintf(`
		SELECT TOP 1 ASSIGNKEY AS parentExtId
		FROM %s.[dbo].[Accounts]
		WHERE AccountKey = @externalUserId
	`, dbName)

	var parent sql.NullString
	err := dbConn.QueryRowContext(ctx, query, sql.Named("externalUserId", userExtID)).Scan(&parent)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if parent.Valid {
		return parent.String, nil
	}
	return "", nil
}

func fetchBaseItems(ctx context.Context, dbConn *sql.DB, dbName string, skus []string) (map[string]*itemState, error) {
	placeholders, args := buildStringInParams("sku", skus)
	query := fmt.Sprintf(`
		SELECT
			i.[ID], i.[ItemKey], i.[Price], i.[DiscountCode],
			i.[VatExampt], i.[DiscountPrc],
			pl.[CurrencyCode], pl.[PriceListNumber]
		FROM %s.[dbo].[Items] AS i
		LEFT JOIN (
			SELECT
				ItemKey, CurrencyCode, PriceListNumber,
				ROW_NUMBER() OVER (
					PARTITION BY ItemKey
					ORDER BY DatF DESC
				) AS rn
			FROM %s.[dbo].[PriceLists]
			WHERE DatF <= GETDATE()
		) AS pl
			ON i.ItemKey = pl.ItemKey AND pl.rn = 1
		WHERE i.ItemKey IN (%s)
	`, dbName, dbName, strings.Join(placeholders, ", "))

	rows, err := dbConn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make(map[string]*itemState, len(skus))
	for rows.Next() {
		var (
			id           int64
			itemKey      string
			price        sql.NullFloat64
			discountCode sql.NullString
			vatExampt    sql.NullBool
			discountPrc  sql.NullFloat64
			currencyCode sql.NullString
			priceListNum sql.NullInt64
		)

		if err := rows.Scan(&id, &itemKey, &price, &discountCode, &vatExampt, &discountPrc, &currencyCode, &priceListNum); err != nil {
			return nil, err
		}

		basePrice := 0.0
		if price.Valid {
			basePrice = price.Float64
		}

		item := &itemState{
			ID:                      id,
			ItemKey:                 itemKey,
			VatExampt:               vatExampt.Valid && vatExampt.Bool,
			DiscountCode:            discountCode.String,
			DiscountPrc:             discountPrc.Float64,
			CurrencyCode:            currencyCode.String,
			Currency:                nonEmpty(currencyCode.String, "N/A"),
			OriginalPrice:           basePrice,
			OriginalPriceNoChange:   basePrice,
			PriceCalculatedNoChange: basePrice,
			Price:                   basePrice,
		}

		if priceListNum.Valid {
			val := int(priceListNum.Int64)
			item.PriceListNumber = &val
		}

		if discountPrc.Valid && discountPrc.Float64 != 0 {
			disc := discountPrc.Float64
			discounted := basePrice - (basePrice*disc)/100
			item.Price = discounted
			item.PriceCalculatedNoChange = discounted
			item.Discount = disc
		}

		items[item.ItemKey] = item
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

func fetchDiscounts(ctx context.Context, dbConn *sql.DB, dbName, accountKey string) ([]discountRecord, error) {
	query := fmt.Sprintf(`
		SELECT PriceListNumber, ItemDiscountCode, DiscountPrc
		FROM %s.[dbo].[Discounts]
		WHERE AccountKey = @accountKey
	`, dbName)

	rows, err := dbConn.QueryContext(ctx, query, sql.Named("accountKey", accountKey))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []discountRecord
	for rows.Next() {
		var (
			plNum    sql.NullInt64
			itemCode sql.NullString
			disc     sql.NullFloat64
		)
		if err := rows.Scan(&plNum, &itemCode, &disc); err != nil {
			return nil, err
		}
		if !plNum.Valid {
			continue
		}
		out = append(out, discountRecord{
			PriceListNumber:  int(plNum.Int64),
			ItemDiscountCode: itemCode.String,
			DiscountPrc:      disc.Float64,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func applyDiscounts(ctx context.Context, dbConn *sql.DB, dbName string, skus []string, items map[string]*itemState, discounts []discountRecord, sourceLabel string, isParent bool) (bool, error) {
	if len(discounts) == 0 {
		return false, nil
	}

	priceListNums := make([]int, 0, len(discounts))
	for _, d := range discounts {
		priceListNums = append(priceListNums, d.PriceListNumber)
	}
	priceListNums = uniqueInts(priceListNums)

	priceLists, err := fetchPriceLists(ctx, dbConn, dbName, skus, priceListNums)
	if err != nil {
		return false, err
	}

	priceListMap := make(map[int]map[string]priceListRecord)
	for _, rec := range priceLists {
		m := priceListMap[rec.PriceListNumber]
		if m == nil {
			m = make(map[string]priceListRecord)
			priceListMap[rec.PriceListNumber] = m
		}
		m[rec.ItemKey] = rec
	}

	userHadAny := false
	for _, disc := range discounts {
		priceMap := priceListMap[disc.PriceListNumber]
		if priceMap == nil {
			continue
		}
		for _, item := range items {
			if item.DiscountFound {
				continue
			}
			priceRec, ok := priceMap[item.ItemKey]
			if !ok {
				continue
			}
			if item.DiscountCode != "" && item.DiscountCode == disc.ItemDiscountCode {
				orgPrice := priceRec.Price
				item.Currency = nonEmpty(priceRec.CurrencyCode, item.Currency)
				item.OriginalPrice = orgPrice
				item.OriginalPriceNoChange = orgPrice
				item.PriceCalculatedNoChange = orgPrice
				item.Price = orgPrice
				item.PriceListGroup = sourceLabel

				plNum := priceRec.PriceListNumber
				if isParent {
					item.ParentPriceListNumber = &plNum
				}
				item.PriceListNumber = &plNum

				userDisc := disc.DiscountPrc
				newP := orgPrice - (orgPrice*userDisc)/100
				item.Price = newP
				item.PriceCalculatedNoChange = newP
				item.Discount = userDisc
				item.DiscountFound = true
				userHadAny = true
			}
		}
	}

	return userHadAny, nil
}

func fetchPriceLists(ctx context.Context, dbConn *sql.DB, dbName string, skus []string, priceListNums []int) ([]priceListRecord, error) {
	if len(priceListNums) == 0 || len(skus) == 0 {
		return nil, nil
	}

	skuPlaceholders, skuArgs := buildStringInParams("sku", skus)
	plPlaceholders, plArgs := buildIntInParams("pl", priceListNums)

	args := append(skuArgs, plArgs...)

	query := fmt.Sprintf(`
		SELECT ItemKey, Price, CurrencyCode, PriceListNumber
		FROM (
			SELECT
				ItemKey, Price, CurrencyCode, PriceListNumber,
				ROW_NUMBER() OVER (
					PARTITION BY ItemKey, PriceListNumber
					ORDER BY DatF DESC
				) AS rn
			FROM %s.[dbo].[PriceLists]
			WHERE ItemKey IN (%s)
			  AND PriceListNumber IN (%s)
		) pl
		WHERE pl.rn = 1
	`, dbName, strings.Join(skuPlaceholders, ", "), strings.Join(plPlaceholders, ", "))

	rows, err := dbConn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []priceListRecord
	for rows.Next() {
		var (
			itemKey         string
			price           sql.NullFloat64
			currency        sql.NullString
			priceListNumber int
		)
		if err := rows.Scan(&itemKey, &price, &currency, &priceListNumber); err != nil {
			return nil, err
		}
		out = append(out, priceListRecord{
			ItemKey:         itemKey,
			Price:           price.Float64,
			CurrencyCode:    currency.String,
			PriceListNumber: priceListNumber,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func fetchSpecialPrices(ctx context.Context, dbConn *sql.DB, dbName string, skus []string, accountKey string) (map[string]specialPriceRecord, error) {
	if accountKey == "" {
		return map[string]specialPriceRecord{}, nil
	}
	placeholders, args := buildStringInParams("sku", skus)
	args = append(args, sql.Named("accountKey", accountKey))

	query := fmt.Sprintf(`
		SELECT sp.ItemKey, sp.Price, sp.DiscountPrc, sp.CurrencyCode
		FROM (
			SELECT
				s1.ItemKey,
				s2.Price,
				s2.DiscountPrc,
				s2.CurrencyCode,
				ROW_NUMBER() OVER (
					PARTITION BY s1.ItemKey
					ORDER BY s1.ValidDate ASC
				) AS rn
			FROM %s.[dbo].[SpecialPrices] AS s1
			JOIN %s.[dbo].[SpecialPricesMoves] AS s2
				ON s1.Id = s2.Spid
			WHERE s1.ItemKey IN (%s)
			  AND s1.AccountKey = @accountKey
			  AND s1.Active = '0'
		) AS sp
		WHERE sp.rn = 1
	`, dbName, dbName, strings.Join(placeholders, ", "))

	rows, err := dbConn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]specialPriceRecord)
	for rows.Next() {
		var (
			itemKey  string
			price    sql.NullFloat64
			disc     sql.NullFloat64
			currency sql.NullString
		)
		if err := rows.Scan(&itemKey, &price, &disc, &currency); err != nil {
			return nil, err
		}
		out[itemKey] = specialPriceRecord{
			ItemKey:      itemKey,
			Price:        price.Float64,
			DiscountPrc:  disc.Float64,
			CurrencyCode: currency.String,
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func fetchLastPrices(ctx context.Context, dbConn *sql.DB, dbName string, skus []string, accountKey string) (map[string]lastPriceRecord, error) {
	if len(skus) == 0 || accountKey == "" {
		return map[string]lastPriceRecord{}, nil
	}

	placeholders, args := buildStringInParams("lp_sku", skus)
	args = append(args, sql.Named("externalUserId", accountKey))

	query := fmt.Sprintf(`
		WITH combined AS (
			SELECT
				CAST([EXPR1] AS NVARCHAR(100)) AS Expr1,
				[CURRENCYCODE],
				[ACCOUNTKEY],
				[ITEMKEY],
				[PRICE],
				[VALUEDATE],
				'invoice' AS Source,
				1 AS src_priority
			FROM %s.[dbo].[ILASTPRICEINV]
			WHERE ITEMKEY IN (%s)
			  AND ACCOUNTKEY = @externalUserId

			UNION ALL

			SELECT
				CAST([EXPR1] AS NVARCHAR(100)) AS Expr1,
				[CURRENCYCODE],
				[ACCOUNTKEY],
				[ITEMKEY],
				[PRICE],
				[VALUEDATE],
				'order' AS Source,
				2 AS src_priority
			FROM %s.[dbo].[ILASTPRICEORDER]
			WHERE ITEMKEY IN (%s)
			  AND ACCOUNTKEY = @externalUserId
		),
		ranked AS (
			SELECT
				Source,
				Expr1,
				CURRENCYCODE AS CurrencyCode,
				ACCOUNTKEY  AS AccountKey,
				ITEMKEY     AS ItemKey,
				PRICE       AS Price,
				VALUEDATE   AS ValueDate,
				ROW_NUMBER() OVER (
					PARTITION BY ITEMKEY
					ORDER BY src_priority ASC, ValueDate DESC, Expr1 DESC
				) AS rn
			FROM combined
		)
		SELECT
			ItemKey,
			Expr1,
			Price,
			CurrencyCode
		FROM ranked
		WHERE rn = 1;
	`, dbName, strings.Join(placeholders, ", "), dbName, strings.Join(placeholders, ", "))

	rows, err := dbConn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]lastPriceRecord)
	for rows.Next() {
		var (
			itemKey  string
			expr1    sql.NullString
			price    sql.NullFloat64
			currency sql.NullString
		)
		if err := rows.Scan(&itemKey, &expr1, &price, &currency); err != nil {
			return nil, err
		}
		lastPrice := parseNullableNumber(expr1.String, price.Float64)
		out[itemKey] = lastPriceRecord{
			ItemKey:   itemKey,
			LastPrice: lastPrice,
			Currency:  nonEmpty(currency.String, "N/A"),
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func fetchStockData(ctx context.Context, dbConn *sql.DB, dbName string, skus, warehouses []string) (map[string]map[string]float64, error) {
	if len(skus) == 0 {
		return map[string]map[string]float64{}, nil
	}
	if len(warehouses) == 0 {
		warehouses = []string{"10"}
	}

	skuPlaceholders, skuArgs := buildStringInParams("sku", skus)
	whPlaceholders, whArgs := buildStringInParams("wh", warehouses)
	args := append(skuArgs, whArgs...)

	query := fmt.Sprintf(`
		SELECT ITEMKEY, WAREHOUSE, SUM(ITEMWARHBAL) AS ITEMWARHBAL
		FROM %s.[dbo].[vBalItemWarehouse]
		WHERE ITEMKEY IN (%s)
		  AND WAREHOUSE IN (%s)
		GROUP BY ITEMKEY, WAREHOUSE
	`, dbName, strings.Join(skuPlaceholders, ", "), strings.Join(whPlaceholders, ", "))

	rows, err := dbConn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]map[string]float64)
	for rows.Next() {
		var (
			itemKey   string
			warehouse string
			bal       sql.NullFloat64
		)
		if err := rows.Scan(&itemKey, &warehouse, &bal); err != nil {
			return nil, err
		}
		if out[itemKey] == nil {
			out[itemKey] = make(map[string]float64)
		}
		out[itemKey][warehouse] = bal.Float64
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func buildStringInParams(prefix string, values []string) ([]string, []any) {
	placeholders := make([]string, 0, len(values))
	args := make([]any, 0, len(values))
	for i, v := range values {
		name := fmt.Sprintf("%s_%d", prefix, i)
		placeholders = append(placeholders, "@"+name)
		args = append(args, sql.Named(name, v))
	}
	return placeholders, args
}

func buildIntInParams(prefix string, values []int) ([]string, []any) {
	placeholders := make([]string, 0, len(values))
	args := make([]any, 0, len(values))
	for i, v := range values {
		name := fmt.Sprintf("%s_%d", prefix, i)
		placeholders = append(placeholders, "@"+name)
		args = append(args, sql.Named(name, v))
	}
	return placeholders, args
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func uniqueInts(values []int) []int {
	seen := make(map[int]struct{}, len(values))
	out := make([]int, 0, len(values))
	for _, v := range values {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Ints(out)
	return out
}

func nonEmpty(val, fallback string) string {
	val = strings.TrimSpace(val)
	if val == "" {
		return fallback
	}
	return val
}

func parseNullableNumber(expr string, fallback float64) float64 {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return fallback
	}
	if v, err := parseFloat(expr); err == nil {
		return v
	}
	return fallback
}

func parseFloat(s string) (float64, error) {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, errors.New("invalid number")
	}
	return v, nil
}

func escapeIdentifier(name string) string {
	name = strings.ReplaceAll(name, "]", "]]")
	return "[" + name + "]"
}
