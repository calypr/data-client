package gecko

import "encoding/json"

type FieldConfig struct {
	Field     string `json:"field,omitempty"`
	DataField string `json:"dataField,omitempty"`
	Index     string `json:"index,omitempty"`
	Label     string `json:"label"`
	Type      string `json:"type,omitempty"`
}

type FilterTab struct {
	Title        string                 `json:"title,omitempty"`
	Fields       []string               `json:"fields"`
	FieldsConfig map[string]FieldConfig `json:"fieldsConfig,omitempty"`
}

type FiltersConfig struct {
	Tabs []FilterTab `json:"tabs"`
}

type TableConfig struct {
	Enabled       bool                          `json:"enabled"`
	Fields        []string                      `json:"fields"`
	Columns       map[string]TableColumnsConfig `json:"columns,omitempty"`
	DetailsConfig TableDetailsConfig            `json:"detailsConfig"`
}

type SummaryTableColumnType string

const (
	SummaryTableColumnTypeString     SummaryTableColumnType = "string"
	SummaryTableColumnTypeNumber     SummaryTableColumnType = "number"
	SummaryTableColumnTypeDate       SummaryTableColumnType = "date"
	SummaryTableColumnTypeArray      SummaryTableColumnType = "array"
	SummaryTableColumnTypeLink       SummaryTableColumnType = "link"
	SummaryTableColumnTypeBoolean    SummaryTableColumnType = "boolean"
	SummaryTableColumnTypeParagraphs SummaryTableColumnType = "paragraphs"
)

type TableColumnsConfig struct {
	Field              string                 `json:"field"`
	Title              string                 `json:"title"`
	AccessorPath       string                 `json:"accessorPath,omitempty"`
	Type               SummaryTableColumnType `json:"type,omitempty"`
	CellRenderFunction string                 `json:"cellRenderFunction,omitempty"`
	Params             map[string]any         `json:"params,omitempty"`
	Width              string                 `json:"width,omitempty"`
	Sortable           bool                   `json:"sortable,omitempty"`
	Visable            bool                   `json:"visable,omitempty"`
}

func (t TableColumnsConfig) MarshalJSON() ([]byte, error) {
	type alias TableColumnsConfig
	aux := struct {
		*alias
		Type *SummaryTableColumnType `json:"type,omitempty"`
	}{
		alias: (*alias)(&t),
	}
	if t.Type != "" {
		aux.Type = &t.Type
	}
	return json.Marshal(aux)
}

type TableDetailsConfig struct {
	Panel       string            `json:"panel,omitempty"`
	Mode        string            `json:"mode,omitempty"`
	IDField     string            `json:"idField,omitempty"`
	FilterField string            `json:"filterField,omitempty"`
	Title       string            `json:"title,omitempty"`
	NodeType    string            `json:"nodeType,omitempty"`
	NodeFields  map[string]string `json:"nodeFields,omitempty"`
}

type GuppyConfig struct {
	DataType                  string              `json:"dataType"`
	NodeCountTitle            string              `json:"nodeCountTitle"`
	FieldMapping              []GuppyFieldMapping `json:"fieldMapping,omitempty"`
	AccessibleFieldCheckList  []string            `json:"accessibleFieldCheckList,omitempty"`
	AccessibleValidationField string              `json:"accessibleValidationField,omitempty"`
	ManifestMapping           ManifestMapping     `json:"manifestMapping"`
}

type GuppyFieldMapping struct {
	Field string `json:"field,omitempty"`
	Name  string `json:"name,omitempty"`
}

type ManifestMapping struct {
	ResourceIndexType               string `json:"resourceIndexType,omitempty"`
	ResourceIdField                 string `json:"resourceIdField,omitempty"`
	ReferenceIdFieldInResourceIndex string `json:"referenceIdFieldInResourceIndex,omitempty"`
	ReferenceIdFieldInDataIndex     string `json:"referenceIdFieldInDataIndex,omitempty"`
}

type Chart struct {
	ChartType string `json:"chartType"`
	Title     string `json:"title"`
}

type ButtonConfig struct {
	Enabled    bool             `json:"enabled,omitempty"`
	Type       string           `json:"type,omitempty"`
	Action     string           `json:"action,omitempty"`
	Title      string           `json:"title,omitempty"`
	LeftIcon   string           `json:"leftIcon,omitempty"`
	RightIcon  string           `json:"rightIcon,omitempty"`
	FileName   string           `json:"fileName,omitempty"`
	ActionArgs ButtonActionArgs `json:"actionArgs"`
}

type ButtonActionArgs struct {
	ResourceIndexType               string   `json:"resourceIndexType,omitempty"`
	ResourceIdField                 string   `json:"resourceIdField,omitempty"`
	ReferenceIdFieldInDataIndex     string   `json:"referenceIdFieldInDataIndex,omitempty"`
	ReferenceIdFieldInResourceIndex string   `json:"referenceIdFieldInResourceIndex,omitempty"`
	FileFields                      []string `json:"fileFields,omitempty"`
}

type ConfigItem struct {
	TabTitle         string           `json:"tabTitle"`
	GuppyConfig      GuppyConfig      `json:"guppyConfig"`
	Charts           map[string]Chart `json:"charts,omitempty"`
	Filters          FiltersConfig    `json:"filters"`
	Table            TableConfig      `json:"table"`
	Dropdowns        map[string]any   `json:"dropdowns,omitempty"`
	Buttons          []ButtonConfig   `json:"buttons,omitempty"`
	LoginForDownload bool             `json:"loginForDownload,omitempty"`
	PreFilters       map[string]any   `json:"preFilters,omitempty"`
}

type Config struct {
	SharedFilters  SharedFiltersConfig `json:"sharedFilters"`
	ExplorerConfig []ConfigItem        `json:"explorerConfig"`
	FileActions    FileActionsConfig   `json:"fileActions,omitempty"`
}

type FileActionsConfig map[string][]string

type SharedFiltersConfig struct {
	SharedFilter map[string][]FilterPair `json:"defined"`
}

type FilterPair struct {
	Index string `json:"index"`
	Field string `json:"field"`
}

type ErrorResponse struct {
	Error HTTPError `json:"error"`
}

func (e ErrorResponse) ErrorMessage() string {
	return e.Error.Message
}

func (e ErrorResponse) ErrorType() string {
	return string(e.Error.Type)
}

func (e ErrorResponse) ErrorDetails() map[string]any {
	return e.Error.Details
}

type HTTPError struct {
	Type    ErrorType      `json:"type,omitempty"`
	Message string         `json:"message"`
	Code    int            `json:"code"`
	Details map[string]any `json:"details,omitempty"`
}

type StatusResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type ConfigType string

const (
	ConfigTypeExplorer    ConfigType = "explorer"
	ConfigTypeNav         ConfigType = "nav"
	ConfigTypeFileSummary ConfigType = "file_summary"
	ConfigTypeAppsPage    ConfigType = "apps_page"
	ConfigTypeProject     ConfigType = "project"
	ConfigTypeProjects    ConfigType = "projects"
)

type ErrorType string

const (
	ErrorTypeNotFound                      ErrorType = "not_found"
	ErrorTypeUnauthorized                  ErrorType = "unauthorized"
	ErrorTypeMethodNotAllowed              ErrorType = "method_not_allowed"
	ErrorTypeMissingAuthorization          ErrorType = "missing_authorization"
	ErrorTypeForbidden                     ErrorType = "forbidden"
	ErrorTypeInvalidConfigType             ErrorType = "invalid_config_type"
	ErrorTypeConfigNotFound                ErrorType = "config_not_found"
	ErrorTypeInvalidJSON                   ErrorType = "invalid_json"
	ErrorTypeEmptyRequestBody              ErrorType = "empty_request_body"
	ErrorTypeInvalidRequestBody            ErrorType = "invalid_request_body"
	ErrorTypeValidationFailed              ErrorType = "validation_failed"
	ErrorTypeMissingProjectID              ErrorType = "missing_project_id"
	ErrorTypeInvalidProjectID              ErrorType = "invalid_project_id"
	ErrorTypeProjectIDMismatch             ErrorType = "project_id_mismatch"
	ErrorTypeInvalidDirectory              ErrorType = "invalid_directory"
	ErrorTypeInvalidUUID                   ErrorType = "invalid_uuid"
	ErrorTypeMissingIdentifier             ErrorType = "missing_identifier"
	ErrorTypeInvalidQueryParameter         ErrorType = "invalid_query_parameter"
	ErrorTypeInvalidDistance               ErrorType = "invalid_distance"
	ErrorTypeInvalidVectorRequest          ErrorType = "invalid_vector_request"
	ErrorTypeInvalidPointData              ErrorType = "invalid_point_data"
	ErrorTypePointNotFound                 ErrorType = "point_not_found"
	ErrorTypeVectorCollectionNotFound      ErrorType = "vector_collection_not_found"
	ErrorTypeVectorCollectionAlreadyExists ErrorType = "vector_collection_already_exists"
	ErrorTypeVectorStoreUnavailable        ErrorType = "vector_store_unavailable"
	ErrorTypeVectorOperationFailed         ErrorType = "vector_operation_failed"
	ErrorTypeDatabaseError                 ErrorType = "database_error"
	ErrorTypeDatabaseUnavailable           ErrorType = "database_unavailable"
	ErrorTypeGraphQueryFailed              ErrorType = "graph_query_failed"
	ErrorTypeInvalidJWTHandler             ErrorType = "invalid_jwt_handler"
	ErrorTypeInvalidAuthorizationResponse  ErrorType = "invalid_authorization_response"
	ErrorTypeAuthorizationServiceError     ErrorType = "authorization_service_error"
	ErrorTypeAppCardNotFound               ErrorType = "app_card_not_found"
)

func KnownConfigTypes() []ConfigType {
	return []ConfigType{
		ConfigTypeExplorer,
		ConfigTypeNav,
		ConfigTypeFileSummary,
		ConfigTypeAppsPage,
		ConfigTypeProject,
		ConfigTypeProjects,
	}
}

type StylingOverrideWithMergeControl struct {
	MergeStrategy string `json:"mergeStrategy,omitempty"`
	Root          string `json:"root,omitempty"`
}

type NavigationButtonProps struct {
	Icon       string                           `json:"icon"`
	Tooltip    string                           `json:"tooltip"`
	Href       string                           `json:"href"`
	NoBasePath *bool                            `json:"noBasePath,omitempty"`
	Name       string                           `json:"name"`
	IconHeight string                           `json:"iconHeight,omitempty"`
	Title      string                           `json:"title,omitempty"`
	ClassNames *StylingOverrideWithMergeControl `json:"classNames,omitempty"`
}

type NavigationBarLogo struct {
	Src         string                           `json:"src"`
	Title       string                           `json:"title,omitempty"`
	Description string                           `json:"description,omitempty"`
	Width       float64                          `json:"width,omitempty"`
	Height      float64                          `json:"height,omitempty"`
	NoBasePath  *bool                            `json:"noBasePath,omitempty"`
	Divider     *bool                            `json:"divider,omitempty"`
	BasePath    string                           `json:"basePath,omitempty"`
	Href        string                           `json:"href"`
	OnToggle    json.RawMessage                  `json:"onToggle,omitempty"`
	Basepage    *bool                            `json:"basepage,omitempty"`
	ClassNames  *StylingOverrideWithMergeControl `json:"classNames,omitempty"`
}

type NavigationProps struct {
	Logo       *NavigationBarLogo               `json:"logo,omitempty"`
	Items      []NavigationButtonProps          `json:"items"`
	Title      string                           `json:"title,omitempty"`
	LoginIcon  json.RawMessage                  `json:"loginIcon,omitempty"`
	ClassNames *StylingOverrideWithMergeControl `json:"classNames"`
}

type LeftNavBarProps struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	Href        string `json:"href"`
	Perms       any    `json:"perms"`
}

type TopBarItem struct {
	ClassNames struct {
		Button string `json:"button"`
		Label  string `json:"label"`
		Root   string `json:"root"`
	} `json:"classNames,omitempty"`
	Href string `json:"href,omitempty"`
	Name string `json:"name,omitempty"`
}

type TopBarProps struct {
	Items                 []TopBarItem `json:"items,omitempty"`
	LoginButtonVisibility string       `json:"loginButtonVisibility,omitempty"`
}

type HeaderProps struct {
	Top        TopBarProps       `json:"topBar"`
	Navigation NavigationProps   `json:"navigation"`
	LeftNav    []LeftNavBarProps `json:"leftnav"`
	BasePage   bool              `json:"basePage,omitempty"`
}

type FooterColumnLink struct {
	Label string `json:"label,omitempty"`
	Href  string `json:"href,omitempty"`
}

type FooterColumn struct {
	Title string             `json:"title,omitempty"`
	Links []FooterColumnLink `json:"links,omitempty"`
}

type FooterRightSection struct {
	Columns []FooterColumn `json:"columns,omitempty"`
}

type FooterProps struct {
	RightSection *FooterRightSection `json:"rightSection,omitempty"`
	BottomLinks  []FooterColumnLink  `json:"bottomLinks,omitempty"`
	ColumnLinks  []FooterColumnLink  `json:"columnLinks,omitempty"`
}

type HeaderMetadata struct {
	Title   string `json:"title"`
	Content string `json:"content"`
	Key     string `json:"key"`
}

type NavPageLayoutProps struct {
	HeaderProps    HeaderProps    `json:"headerProps"`
	FooterProps    FooterProps    `json:"footerProps"`
	HeaderMetadata HeaderMetadata `json:"headerMetadata"`
}

type FilesummaryConfig struct {
	Config         map[string]TableColumnsConfig `json:"config"`
	BarChartColor  string                        `json:"barChartColor"`
	DefaultProject string                        `json:"defaultProject"`
	BinslicePoints []int                         `json:"binslicePoints"`
	IdField        string                        `json:"idField"`
	Index          string                        `json:"index"`
}

type AppCard struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	Href        string `json:"href"`
	Perms       string `json:"perms"`
}

type AppsConfig struct {
	AppCards []AppCard `json:"appCards"`
}

type ProjectConfig struct {
	Title        string `json:"title"`
	ContactEmail string `json:"contact_email"`
	SrcRepo      string `json:"src_repo"`
	OrgTitle     string `json:"org_title"`
	Description  string `json:"description"`
	ProjectTitle string `json:"project_title"`
	IconName     string `json:"icon_name"`
}
