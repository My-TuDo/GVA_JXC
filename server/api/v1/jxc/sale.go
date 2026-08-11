package jxc

import (
	"fmt"

	"github.com/flipped-aurora/gin-vue-admin/server/global"
	"github.com/flipped-aurora/gin-vue-admin/server/model/common/request"
	"github.com/flipped-aurora/gin-vue-admin/server/model/common/response"
	"github.com/flipped-aurora/gin-vue-admin/server/model/jxc"
	"github.com/flipped-aurora/gin-vue-admin/server/utils"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// SaleApi 销售中心接口
type SaleApi struct{}

// saleOperator 当前操作人
func saleOperator(c *gin.Context) string {
	return fmt.Sprintf("%d", utils.GetUserID(c))
}

// GetSalePage 分页查询销售单
// @Tags     JxcSale
// @Summary  分页查询销售单列表（支持日期范围过滤）
// @Security ApiKeyAuth
// @Produce  application/json
// @Param    page     query     int     true  "页码"
// @Param    pageSize query     int     true  "每页条数"
// @Param    keyword  query     string  false "搜索单号"
// @Param    from     query     string  false "起始日期 yyyy-mm-dd（含当天）"
// @Param    to       query     string  false "结束日期 yyyy-mm-dd（含当天）"
// @Success  200      {object}  response.PageResult{list=[]jxc.SaleOrder,total=int64}  "销售单列表"
// @Router   /jxc/sale/page [get]
func (a *SaleApi) GetSalePage(c *gin.Context) {
	var q jxc.SaleQuery
	_ = c.ShouldBindQuery(&q)
	list, total, err := saleService.GetSalePage(c.Request.Context(), q)
	if err != nil {
		global.GVA_LOG.Error("获取销售单列表失败!", zap.Error(err))
		response.FailWithMessage("获取销售单列表失败", c)
		return
	}
	response.OkWithData(response.PageResult{List: list, Total: total, Page: q.Page, PageSize: q.PageSize}, c)
}

// GetSaleDetail 查询销售单详情（含明细）
// @Tags     JxcSale
// @Summary  查询销售单详情（含明细）
// @Security ApiKeyAuth
// @Produce  application/json
// @Param    id   query  uint  true  "销售单ID"
// @Success  200  {object}  response.Response{data=jxc.SaleOrder}  "销售单详情"
// @Router   /jxc/sale/detail [get]
func (a *SaleApi) GetSaleDetail(c *gin.Context) {
	var req request.GetById
	if err := c.ShouldBindQuery(&req); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	order, err := saleService.GetSaleDetail(c.Request.Context(), req.Uint())
	if err != nil {
		global.GVA_LOG.Error("获取销售单详情失败!", zap.Error(err))
		response.FailWithMessage("获取销售单详情失败", c)
		return
	}
	response.OkWithData(order, c)
}

// GetSaleRemaining 查询原单剩余可退换数量（按商品，供前端数量上限钳制）
// @Tags     JxcSale
// @Summary  查询原单剩余可退换数量
// @Security ApiKeyAuth
// @Produce  application/json
// @Param    id   query  uint  true  "原单ID"
// @Success  200  {object}  response.Response{data=[]jxc.SaleRemaining}  "剩余可退换数量"
// @Router   /jxc/sale/remaining [get]
func (a *SaleApi) GetSaleRemaining(c *gin.Context) {
	var req request.GetById
	if err := c.ShouldBindQuery(&req); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	list, err := saleService.GetRemaining(c.Request.Context(), req.Uint())
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	response.OkWithData(list, c)
}

// CreateSaleOrder 创建销售单
// @Tags     JxcSale
// @Summary  创建销售单（正常销售自动锁定库存）
// @Security ApiKeyAuth
// @accept   application/json
// @Produce  application/json
// @Param    data body      model/jxc.SaleOrder  true "销售单信息（含 items）"
// @Success  200  {object}  response.Response{msg=string}  "创建销售单"
// @Router   /jxc/sale [post]
func (a *SaleApi) CreateSaleOrder(c *gin.Context) {
	var order jxc.SaleOrder
	if err := c.ShouldBindJSON(&order); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	order.Creator = saleOperator(c)
	if err := saleService.CreateSaleOrder(c.Request.Context(), &order); err != nil {
		global.GVA_LOG.Error("创建销售单失败!", zap.Error(err))
		response.FailWithMessage("创建失败："+err.Error(), c)
		return
	}
	response.OkWithDetailed(&order, "创建成功", c)
}

// UpdateSaleOrder 更新销售单（仅待出库）
// @Tags     JxcSale
// @Summary  更新销售单（仅待出库, 明细重建并重新锁定库存）
// @Security ApiKeyAuth
// @accept   application/json
// @Produce  application/json
// @Param    data body      model/jxc.SaleOrder  true "销售单信息（需含ID与items）"
// @Success  200  {object}  response.Response{msg=string}  "更新销售单"
// @Router   /jxc/sale [put]
func (a *SaleApi) UpdateSaleOrder(c *gin.Context) {
	var order jxc.SaleOrder
	if err := c.ShouldBindJSON(&order); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	if err := saleService.UpdateSaleOrder(c.Request.Context(), &order); err != nil {
		global.GVA_LOG.Error("更新销售单失败!", zap.Error(err))
		response.FailWithMessage("更新失败："+err.Error(), c)
		return
	}
	response.OkWithMessage("更新成功", c)
}

// DeleteSaleOrder 删除销售单（仅待出库）
// @Tags     JxcSale
// @Summary  删除销售单（仅待出库, 释放锁定库存）
// @Security ApiKeyAuth
// @accept   application/json
// @Produce  application/json
// @Param    data body      request.GetById  true "销售单ID"
// @Success  200  {object}  response.Response{msg=string}  "删除销售单"
// @Router   /jxc/sale [delete]
func (a *SaleApi) DeleteSaleOrder(c *gin.Context) {
	var req request.GetById
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	if err := saleService.DeleteSaleOrder(c.Request.Context(), req.Uint()); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	response.OkWithMessage("删除成功", c)
}

// CancelSaleOrder 取消销售单
// @Tags     JxcSale
// @Summary  取消销售单（待出库 → 已取消, 释放锁定库存）
// @Security ApiKeyAuth
// @accept   application/json
// @Produce  application/json
// @Param    data body      request.GetById  true "销售单ID"
// @Success  200  {object}  response.Response{msg=string}  "已取消"
// @Router   /jxc/sale/cancel [put]
func (a *SaleApi) CancelSaleOrder(c *gin.Context) {
	var req request.GetById
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	if err := saleService.CancelSaleOrder(c.Request.Context(), req.Uint()); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	response.OkWithMessage("已取消", c)
}

// ConfirmOut 确认出库
// @Tags     JxcSale
// @Summary  确认出库（扣减库存并写流水）
// @Security ApiKeyAuth
// @accept   application/json
// @Produce  application/json
// @Param    data body      request.GetById  true "销售单ID"
// @Success  200  {object}  response.Response{msg=string}  "出库成功"
// @Router   /jxc/sale/out [put]
func (a *SaleApi) ConfirmOut(c *gin.Context) {
	var req request.GetById
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	if err := saleService.ConfirmOut(c.Request.Context(), req.Uint(), saleOperator(c)); err != nil {
		global.GVA_LOG.Error("销售出库失败!", zap.Error(err))
		response.FailWithMessage("出库失败："+err.Error(), c)
		return
	}
	response.OkWithMessage("出库成功", c)
}

// ConfirmReturn 确认退货入库
// @Tags     JxcSale
// @Summary  确认退货入库（恢复库存并写流水）
// @Security ApiKeyAuth
// @accept   application/json
// @Produce  application/json
// @Param    data body      request.GetById  true "退货单ID"
// @Success  200  {object}  response.Response{msg=string}  "退货入库成功"
// @Router   /jxc/sale/return [put]
func (a *SaleApi) ConfirmReturn(c *gin.Context) {
	var req request.GetById
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	if err := saleService.ConfirmReturn(c.Request.Context(), req.Uint(), saleOperator(c)); err != nil {
		global.GVA_LOG.Error("退货入库失败!", zap.Error(err))
		response.FailWithMessage("退货失败："+err.Error(), c)
		return
	}
	response.OkWithMessage("退货入库成功", c)
}

// ConfirmExchange 确认换货
// @Tags     JxcSale
// @Summary  确认换货（换出扣库存/换入加库存）
// @Security ApiKeyAuth
// @accept   application/json
// @Produce  application/json
// @Param    data body      request.GetById  true "换货单ID"
// @Success  200  {object}  response.Response{msg=string}  "换货确认成功"
// @Router   /jxc/sale/exchange [put]
func (a *SaleApi) ConfirmExchange(c *gin.Context) {
	var req request.GetById
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	if err := saleService.ConfirmExchange(c.Request.Context(), req.Uint(), saleOperator(c)); err != nil {
		global.GVA_LOG.Error("换货确认失败!", zap.Error(err))
		response.FailWithMessage("换货失败："+err.Error(), c)
		return
	}
	response.OkWithMessage("换货确认成功", c)
}
