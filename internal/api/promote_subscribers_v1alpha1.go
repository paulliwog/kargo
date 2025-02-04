package api

import (
	"context"
	goerrors "errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kargoapi "github.com/akuity/kargo/api/v1alpha1"
	typesv1alpha1 "github.com/akuity/kargo/internal/api/types/v1alpha1"
	"github.com/akuity/kargo/internal/kargo"
	svcv1alpha1 "github.com/akuity/kargo/pkg/api/service/v1alpha1"
	"github.com/akuity/kargo/pkg/api/v1alpha1"
)

// PromoteSubscribers creates a Promotion resources to transition all Stages
// immediately downstream from the specified Stage into the state represented by
// the specified Freight.
func (s *server) PromoteSubscribers(
	ctx context.Context,
	req *connect.Request[svcv1alpha1.PromoteSubscribersRequest],
) (*connect.Response[svcv1alpha1.PromoteSubscribersResponse], error) {
	if err := validateProjectAndStageNonEmpty(req.Msg.GetProject(), req.Msg.GetStage()); err != nil {
		return nil, err
	}
	if err := s.validateProjectFn(ctx, req.Msg.GetProject()); err != nil {
		return nil, err
	}

	stage, err := s.getStageFn(
		ctx,
		s.client,
		types.NamespacedName{
			Namespace: req.Msg.GetProject(),
			Name:      req.Msg.GetStage(),
		},
	)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if stage == nil {
		return nil, connect.NewError(
			connect.CodeNotFound,
			errors.Errorf(
				"Stage %q not found in namespace %q",
				req.Msg.GetStage(),
				req.Msg.GetProject(),
			),
		)
	}

	// Get the specified Freight, but only if it has qualified for this Stage.
	// Expect a nil if it is either not found or is not qualified . Errors are
	// internal problems.
	var freight *kargoapi.Freight
	if freight, err = s.getQualifiedFreightFn(
		ctx,
		s.client,
		types.NamespacedName{
			Namespace: req.Msg.GetProject(),
			Name:      req.Msg.GetFreight(),
		},
		[]string{req.Msg.GetStage()},
	); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	} else if freight == nil {
		return nil, connect.NewError(
			connect.CodeNotFound,
			errors.Errorf(
				"no qualified Freight %q found in namespace %q",
				req.Msg.GetFreight(),
				req.Msg.GetProject(),
			),
		)
	}

	subscribers, err := s.findStageSubscribersFn(ctx, stage)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if len(subscribers) == 0 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("Stage %q has no subscribers", req.Msg.GetStage()))
	}

	promoteErrs := make([]error, 0, len(subscribers))
	createdPromos := make([]*v1alpha1.Promotion, 0, len(subscribers))
	for _, subscriber := range subscribers {
		newPromo := kargo.NewPromotion(subscriber, req.Msg.GetFreight())
		if err := s.createPromotionFn(ctx, &newPromo); err != nil {
			promoteErrs = append(promoteErrs, err)
			continue
		}
		createdPromos = append(createdPromos, typesv1alpha1.ToPromotionProto(newPromo))
	}

	res := connect.NewResponse(&svcv1alpha1.PromoteSubscribersResponse{
		Promotions: createdPromos,
	})

	if len(promoteErrs) > 0 {
		return res,
			connect.NewError(connect.CodeInternal, goerrors.Join(promoteErrs...))
	}

	return res, nil
}

// findStageSubscribers returns a list of Stages that are subscribed to the given Stage
// TODO: this could be powered by an index.
func (s *server) findStageSubscribers(ctx context.Context, stage *kargoapi.Stage) ([]kargoapi.Stage, error) {
	var allStages kargoapi.StageList
	if err := s.client.List(ctx, &allStages, client.InNamespace(stage.Namespace)); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	var subscribers []kargoapi.Stage
	for _, s := range allStages.Items {
		s := s
		if s.Spec.Subscriptions == nil {
			continue
		}
		for _, upstream := range s.Spec.Subscriptions.UpstreamStages {
			if upstream.Name != stage.Name {
				continue
			}
			subscribers = append(subscribers, s)
		}
	}
	return subscribers, nil
}
