package bilibili

import (
	"github.com/Sora233/DDBOT/lsp/concern"
	"github.com/Sora233/DDBOT/utils"
	"strconv"
	"strings"
)

type GroupConcernConfig struct {
	concern.IConfig
	StateManager *StateManager
}

func (g *GroupConcernConfig) NotifyBeforeCallback(inotify concern.Notify) {
	if inotify.Type() != News {
		return
	}
	notify := inotify.(*ConcernNewsNotify)
	if notify.Card.GetDesc().GetType() != DynamicDescType_WithVideo {
		return
	}
	videoCard, err := notify.Card.GetCardWithVideo()
	if err != nil {
		return
	}
	videoOrigin := videoCard.GetOrigin()
	if videoOrigin == nil {
		return
	}

	// 主要是解决联合投稿的时候刷屏

	notify.videoOrigin = videoOrigin

	err = g.StateManager.SetGroupVideoOriginMarkIfNotExist(notify.GetGroupCode(), videoOrigin.GetBvid())
	if err == nil {
		notify.videoOriginMark = true
	}
}

func (g *GroupConcernConfig) AtBeforeHook(notify concern.Notify) (hook *concern.HookResult) {
	hook = new(concern.HookResult)
	switch e := notify.(type) {
	case *ConcernLiveNotify:
		if !e.Living() {
			hook.Reason = "Living() is false"
		} else if !notify.(*ConcernLiveNotify).LiveStatusChanged {
			hook.Reason = "Living() ok but LiveStatusChanged is false"
		} else {
			hook.Pass = true
		}
	case *ConcernNewsNotify:
		hook.Pass = true
	default:
		hook.Reason = "default"
	}
	return
}

func (g *GroupConcernConfig) ShouldSendHook(notify concern.Notify) (hook *concern.HookResult) {
	hook = new(concern.HookResult)
	switch e := notify.(type) {
	case *ConcernLiveNotify:
		if e.Living() {
			if e.LiveStatusChanged {
				// 上播了
				hook.Pass = true
				return
			}
			if e.LiveTitleChanged {
				// 直播间标题改了，检查改标题推送配置
				hook.PassOrReason(g.GetGroupConcernNotify().CheckTitleChangeNotify(notify.Type()), "CheckTitleChangeNotify is false")
				return
			}
		} else {
			if e.LiveStatusChanged {
				// 下播了，检查下播推送配置
				hook.PassOrReason(g.GetGroupConcernNotify().CheckOfflineNotify(notify.Type()), "CheckOfflineNotify is false")
				return
			}
		}
		return g.IConfig.ShouldSendHook(notify)
	case *ConcernNewsNotify:
		hook.Pass = true
		return
	}
	return g.IConfig.ShouldSendHook(notify)
}

func (g *GroupConcernConfig) FilterHook(notify concern.Notify) (hook *concern.HookResult) {
	hook = new(concern.HookResult)
	switch n := notify.(type) {
	case *ConcernLiveNotify:
		hook.Pass = true
		return
	case *ConcernNewsNotify:
		// 2021-08-15 发现好像是系统推荐的直播间，非人为操作，选择不推送
		if n.Card.GetDesc().GetType() == DynamicDescType_WithLiveV2 {
			hook.Reason = "WithLiveV2 news notify filtered"
			return
		}

		// 没设置过滤，pass
		if g.GetGroupConcernFilter().Empty() {
			hook.Pass = true
			return
		}

		logger := notify.Logger().WithField("FilterType", g.GetGroupConcernFilter().Type)
		switch g.GetGroupConcernFilter().Type {
		case concern.FilterTypeText:
			textFilter, err := g.GetGroupConcernFilter().GetFilterByText()
			if err != nil {
				logger.WithField("GroupConcernTextFilter", g.GetGroupConcernFilter().Config).
					Errorf("get text filter error %v", err)
				hook.Pass = true
			} else {
				for _, text := range textFilter.Text {
					if strings.Contains(utils.MsgToString(notify.ToMessage()), text) {
						hook.Pass = true
						break
					}
				}
				if !hook.Pass {
					logger.WithField("TextFilter", textFilter.Text).
						Debug("news notify filtered by textFilter")
					hook.Reason = "TextFilter All pattern match failed"
				} else {
					logger.Debugf("news notify FilterHook pass")
				}
			}
		case concern.FilterTypeType, concern.FilterTypeNotType:
			typeFilter, err := g.GetGroupConcernFilter().GetFilterByType()
			if err != nil {
				logger.WithField("GroupConcernFilterConfig", g.GetGroupConcernFilter().Config).
					Errorf("get type filter error %v", err)
				hook.Pass = true
			} else {
				var convTypes []DynamicDescType
				for _, tp := range typeFilter.Type {
					if types, _ := PredefinedType[tp]; types != nil {
						convTypes = append(convTypes, types...)
					} else {
						if t, err := strconv.ParseInt(tp, 10, 32); err == nil {
							convTypes = append(convTypes, DynamicDescType(t))
						}
					}
				}

				var ok bool
				switch g.GetGroupConcernFilter().Type {
				case concern.FilterTypeType:
					ok = false
					for _, tp := range convTypes {
						if n.Card.GetDesc().GetType() == tp {
							ok = true
							break
						}
					}
				case concern.FilterTypeNotType:
					ok = true
					for _, tp := range convTypes {
						if n.Card.GetDesc().GetType() == tp {
							ok = false
							break
						}
					}
				}
				if ok {
					logger.Debugf("news notify FilterHook pass")
					hook.Pass = true
				} else {
					logger.WithField("TypeFilter", convTypes).
						Debug("news notify FilterHook filtered")
					hook.Reason = "filtered by TypeFilter"
				}
			}
		default:
			hook.Reason = "unknown filter type"
		}
		return
	default:
		hook.Reason = "unknown notify type"
		return
	}
}

func NewGroupConcernConfig(g concern.IConfig, sm *StateManager) *GroupConcernConfig {
	return &GroupConcernConfig{g, sm}
}

const (
	Zhuanlan      = "专栏"
	Zhuanfa       = "转发"
	Tougao        = "投稿"
	Wenzi         = "文字"
	Tupian        = "图片"
	Zhibofenxiang = "直播分享"
)

var PredefinedType = map[string][]DynamicDescType{
	Zhuanlan:      {DynamicDescType_WithPost},
	Zhuanfa:       {DynamicDescType_WithOrigin},
	Tougao:        {DynamicDescType_WithVideo, DynamicDescType_WithMusic, DynamicDescType_WithPost},
	Wenzi:         {DynamicDescType_TextOnly},
	Tupian:        {DynamicDescType_WithImage},
	Zhibofenxiang: {DynamicDescType_WithLive, DynamicDescType_WithLiveV2},
}

func CheckTypeDefine(types []string) (invalid []string) {
	for _, t := range types {
		if PredefinedType[t] != nil {
			continue
		}
		tp, err := strconv.ParseInt(t, 10, 32)
		if err != nil {
			invalid = append(invalid, t)
			continue
		}
		if _, found := DynamicDescType_name[int32(tp)]; tp != 0 && found {
			continue
		}
		invalid = append(invalid, t)
	}
	return
}
